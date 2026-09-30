package ntfs

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// RecordMagic marks the start of a valid FILE record header.
const RecordMagic = "FILE"

// ErrBadRecordMagic reports bytes that are not an MFT record header.
var ErrBadRecordMagic = errors.New("ntfs: record does not start with the file signature")

// The fixed system files occupy the first records of the $mft, in this order.
// They are untyped constants so callers can pass them as either int64 record
// indexes or uint64 identifiers without a conversion at every use site.
const (
	MFT       = 0
	MFTMirror = 1
	LogFile   = 2
	Volume    = 3
	AttrDef   = 4
	RootDir   = 5
	Bitmap    = 6
	Boot      = 7
	BadClus   = 8
	Secure    = 9
	UpCase    = 10
	Extend    = 11

	// FixedSystemFiles is the count of the system records defined above.
	FixedSystemFiles = 12
)

// RecordFlags is the bit set of the MFT record header flags field.
type RecordFlags uint16

const (
	// FlagInUse marks a record currently allocated to a file.
	FlagInUse RecordFlags = 0x0001
	// FlagDirectory marks a record describing a directory.
	FlagDirectory RecordFlags = 0x0002
)

// RecordHeader is the decoded FILE header at the start of every MFT record.
type RecordHeader struct {
	// Magic is the "FILE" signature; always RecordMagic once decoded.
	Magic string
	// FixupOffset is the byte offset of the update sequence array.
	FixupOffset uint16
	// FixupCount is the number of words covered by the update sequence.
	FixupCount uint16
	// SequenceNumber increments each time the record is reused.
	SequenceNumber uint16
	// HardLinkCount is the number of directory entries pointing at the record.
	HardLinkCount uint16
	// AttributesOffset is where the attribute list starts.
	AttributesOffset uint16
	// Flags is the record state bit set.
	Flags RecordFlags
	// BytesInUse is the number of bytes of the record that carry data.
	BytesInUse uint32
	// BytesAllocated is the size of the record on disk.
	BytesAllocated uint32
	// BaseRecord is the record number this record extends, zero when it does not.
	BaseRecord uint64
}

// IsInUse reports whether the record is allocated to a file.
func (h RecordHeader) IsInUse() bool { return h.Flags&FlagInUse != 0 }

// IsDirectory reports whether the record describes a directory.
func (h RecordHeader) IsDirectory() bool { return h.Flags&FlagDirectory != 0 }

// DecodeRecordHeader decodes the FILE header of one MFT record.
func DecodeRecordHeader(b []byte) (RecordHeader, error) {
	// The header is 32 bytes; the update sequence array follows it.
	const headerSize = 0x20

	if len(b) < headerSize {
		return RecordHeader{}, fmt.Errorf("ntfs: mft record truncated at %d bytes", len(b))
	}

	h := RecordHeader{
		Magic:            string(b[0:4]),
		FixupOffset:      binary.LittleEndian.Uint16(b[4:6]),
		FixupCount:       binary.LittleEndian.Uint16(b[6:8]),
		SequenceNumber:   binary.LittleEndian.Uint16(b[0x10:0x12]),
		HardLinkCount:    binary.LittleEndian.Uint16(b[0x12:0x14]),
		AttributesOffset: binary.LittleEndian.Uint16(b[0x14:0x16]),
		Flags:            RecordFlags(binary.LittleEndian.Uint16(b[0x16:0x18])),
		BytesInUse:       binary.LittleEndian.Uint32(b[0x18:0x1C]),
		BytesAllocated:   binary.LittleEndian.Uint32(b[0x1C:0x20]),
		BaseRecord:       binary.LittleEndian.Uint64(b[0x20:0x28]),
	}

	if h.Magic != RecordMagic {
		return h, fmt.Errorf("%w: magic %q", ErrBadRecordMagic, h.Magic)
	}

	if int(h.AttributesOffset) > len(b) || h.AttributesOffset < headerSize {
		return h, fmt.Errorf("ntfs: attribute offset %d outside the record", h.AttributesOffset)
	}

	if int(h.BytesInUse) > len(b) {
		return h, fmt.Errorf("ntfs: record claims %d bytes in use, buffer holds %d", h.BytesInUse, len(b))
	}

	return h, nil
}

// SystemName reports the fixed NTFS name of a system record, or an empty string
// when index is not one of the reserved records.
func SystemName(index uint64) string {
	switch index {
	case MFT:
		return "$mft"
	case MFTMirror:
		return "$mftmirr"
	case LogFile:
		return "$logfile"
	case Volume:
		return "$volume"
	case AttrDef:
		return "$attrdef"
	case RootDir:
		return "."
	case Bitmap:
		return "$bitmap"
	case Boot:
		return "$boot"
	case BadClus:
		return "$badclus"
	case Secure:
		return "$secure"
	case UpCase:
		return "$upcase"
	case Extend:
		return "$extend"
	default:
		return ""
	}
}

// MirrorRecordCount reports how many leading records $mftmirr replicates. NTFS
// fixes the mirrored set at the first four records regardless of volume size.
const MirrorRecordCount = 4
