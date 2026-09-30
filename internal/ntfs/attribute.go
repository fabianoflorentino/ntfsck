package ntfs

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// AttributeType is the on-disk type code of an MFT attribute.
type AttributeType uint32

const (
	// TypeStandardInformation is $STANDARD_INFORMATION.
	TypeStandardInformation AttributeType = 0x10
	// TypeFileName is $FILE_NAME.
	TypeFileName AttributeType = 0x30
	// TypeData is $DATA, the unnamed payload of a file.
	TypeData AttributeType = 0x80
)

// ErrAttributeNotFound reports an attribute type absent from a record.
var ErrAttributeNotFound = errors.New("ntfs: attribute type not present in record")

// Attribute is the decoded common header of one MFT attribute.
type Attribute struct {
	// Offset is the attribute's byte position within the record.
	Offset int
	// Type is the attribute type code.
	Type AttributeType
	// Length is the total on-disk length of the attribute, header included.
	Length uint32
	// IsNonResident reports whether the value lives outside the record.
	IsNonResident bool
	// NameLength is the length of the attribute name in bytes, zero for unnamed.
	NameLength uint8
	// Flags is the attribute state bit set.
	Flags uint16
	// AttributeID identifies the attribute within its record.
	AttributeID uint16
	// StartVCN and LastVCN delimit the covered cluster range of a
	// non-resident value; both are zero for a resident value.
	StartVCN uint64
	LastVCN  uint64
	// RunlistOffset locates the runlist of a non-resident value, relative to
	// the start of the attribute header.
	RunlistOffset uint16
	// CompressionUnitSize is meaningful only for compressed values.
	CompressionUnitSize uint16
	// AllocatedSize is the space the value occupies on disk, in bytes.
	AllocatedSize int64
	// ContentSize is the logical length of the value in bytes.
	ContentSize int64
}

// AttributeIterator walks the attribute list of a record, yielding each
// attribute header in on-disk order.
type AttributeIterator struct {
	record []byte
	cursor int
}

// Attributes returns an iterator positioned at the record's attribute list.
func Attributes(record []byte, header RecordHeader) *AttributeIterator {
	return &AttributeIterator{record: record, cursor: int(header.AttributesOffset)}
}

// Next decodes the attribute at the cursor and advances it. It returns false
// once the list is exhausted or the next header would run past the record.
func (it *AttributeIterator) Next() (Attribute, bool) {
	if it.cursor+8 > len(it.record) {
		return Attribute{}, false
	}

	// 0xFFFFFFFF marks the end of the attribute list in place of a length.
	if binary.LittleEndian.Uint32(it.record[it.cursor:it.cursor+4]) == ^uint32(0) {
		return Attribute{}, false
	}

	attr, err := decodeAttribute(it.record, it.cursor)
	if err != nil {
		return Attribute{}, false
	}

	it.cursor += int(attr.Length)

	return attr, true
}

func decodeAttribute(record []byte, at int) (Attribute, error) {
	if at+16 > len(record) {
		return Attribute{}, errors.New("ntfs: attribute header truncated")
	}

	attr := Attribute{
		Offset:        at,
		Type:          AttributeType(binary.LittleEndian.Uint32(record[at : at+4])),
		Length:        binary.LittleEndian.Uint32(record[at+4 : at+8]),
		IsNonResident: record[at+8] != 0,
		NameLength:    record[at+9],
		Flags:         binary.LittleEndian.Uint16(record[at+10 : at+12]),
		AttributeID:   binary.LittleEndian.Uint16(record[at+12 : at+14]),
	}

	if attr.Length < 16 || at+int(attr.Length) > len(record) {
		return Attribute{}, fmt.Errorf("ntfs: attribute length %d does not fit the record", attr.Length)
	}

	if attr.IsNonResident {
		// A non-resident header is 64 bytes before the runlist starts.
		const nonResidentHeaderSize = 0x40

		if at+nonResidentHeaderSize > len(record) {
			return Attribute{}, errors.New("ntfs: non-resident attribute header truncated")
		}

		attr.StartVCN = binary.LittleEndian.Uint64(record[at+0x10 : at+0x18])
		attr.LastVCN = binary.LittleEndian.Uint64(record[at+0x18 : at+0x20])
		attr.RunlistOffset = binary.LittleEndian.Uint16(record[at+0x20 : at+0x22])
		attr.CompressionUnitSize = binary.LittleEndian.Uint16(record[at+0x22 : at+0x24])
		attr.AllocatedSize = int64(binary.LittleEndian.Uint64(record[at+0x28 : at+0x30]))
		attr.ContentSize = int64(binary.LittleEndian.Uint64(record[at+0x38 : at+0x40]))

		if int(attr.RunlistOffset) < nonResidentHeaderSize || int(attr.RunlistOffset) >= int(attr.Length) {
			return Attribute{}, fmt.Errorf("ntfs: runlist offset %d outside the attribute", attr.RunlistOffset)
		}

		return attr, nil
	}

	// A resident header is 24 bytes; the value length sits at 0x10.
	const residentHeaderSize = 0x18

	if at+residentHeaderSize > len(record) {
		return Attribute{}, errors.New("ntfs: resident attribute header truncated")
	}

	attr.ContentSize = int64(binary.LittleEndian.Uint32(record[at+0x10 : at+0x14]))

	return attr, nil
}

// FindAttribute returns the first attribute of the given type in a record.
func FindAttribute(record []byte, header RecordHeader, want AttributeType) (Attribute, error) {
	it := Attributes(record, header)
	for {
		attr, ok := it.Next()
		if !ok {
			return Attribute{}, fmt.Errorf("%w: %#x", ErrAttributeNotFound, uint32(want))
		}

		if attr.Type == want {
			return attr, nil
		}
	}
}

// DataRuns decodes the runlist of the unnamed $data attribute of a record and
// returns its runs together with the logical content size in bytes.
func DataRuns(record []byte) ([]Run, int64, error) {
	header, err := DecodeRecordHeader(record)
	if err != nil {
		return nil, 0, err
	}

	attr, err := FindAttribute(record, header, TypeData)
	if err != nil {
		return nil, 0, err
	}

	if !attr.IsNonResident {
		return nil, attr.ContentSize, errors.New("ntfs: $data attribute is resident, no runlist to follow")
	}

	// The runlist offset is relative to the start of the attribute header.
	runlist := record[attr.Offset+int(attr.RunlistOffset):]

	runs, _, err := ParseRunList(runlist, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("ntfs: decoding $data runlist: %w", err)
	}

	return runs, attr.ContentSize, nil
}
