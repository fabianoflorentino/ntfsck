// Package volumeimg builds synthetic NTFS volumes in memory so the checker and
// the repair engine can be tested against known-good and deliberately corrupt
// on-disk structures without touching a real medium.
package volumeimg

import (
	"encoding/binary"
	"io"

	"github.com/fabianoflorentino/ntfsck/internal/device"
	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
)

// Geometry of every image the suite builds. A cluster spans four MFT records,
// which keeps the byte offsets in the fixtures easy to reason about.
const (
	BytesPerSector    = 512
	SectorsPerCluster = 8
	ClusterSize       = BytesPerSector * SectorsPerCluster
	RecordsPerCluster = ClusterSize / ntfs.RecordSize

	// MFTCluster is the $mft zone cluster, where record 0 lives.
	MFTCluster = 4
	// MFTClusters is the length of the $mft in clusters.
	MFTClusters = 8
	// RecordCount is how many MFT records the image declares.
	RecordCount = MFTClusters * RecordsPerCluster
	// MirrorCluster is where the $mftmirr payload lives.
	MirrorCluster = 64
	// VolumeClusters is the size of the image in clusters.
	VolumeClusters = 4096

	// attributesAt is where the attribute list starts in every fixture record.
	attributesAt = 0x38
	// runlistAt is where a fixture attribute keeps its runlist.
	runlistAt = 0x40
	// nonResidentHeaderSize is the size of a non-resident attribute header.
	nonResidentHeaderSize = 0x40
)

// Bytes is a synthetic NTFS volume.
type Bytes struct {
	data []byte
}

// New returns an image with a valid boot sector, a $mft whose records are all
// in use, and a $mftmirr payload that matches $mft exactly.
func New() *Bytes {
	img := &Bytes{data: make([]byte, VolumeClusters*ClusterSize)}

	img.writeBootSector()
	for index := range int64(RecordCount) {
		img.writeRecord(index, img.newRecord(ntfs.FlagInUse))
	}

	// Record 0 describes the $mft extent and record 1 becomes $mftmirr. Both
	// must be in place before the payload is copied, so the mirrored copy of
	// record 1 is the real $mftmirr record.
	img.writeRecord(ntfs.MFT, img.newDataRecord(MFTClusters, MFTCluster, int64(MFTClusters)*ClusterSize))
	img.writeRecord(ntfs.MFTMirror, img.newMirrorRecord())
	img.writeMirrorPayload()

	return img
}

// NewDiverged returns an image whose $mftmirr copy of record 3 no longer
// matches $mft record 3, reproducing the inconsistency that motivated the tool.
func NewDiverged() *Bytes {
	img := New()

	// Corrupt only the mirrored copy; $mft stays authoritative.
	offset := int64(MirrorCluster*ClusterSize) + (ntfs.MirrorRecordCount-1)*ntfs.RecordSize
	binary.LittleEndian.PutUint16(img.data[offset:], 0xDEAD)

	return img
}

// NewEmpty returns a readable but non-NTFS image.
func NewEmpty() *Bytes {
	return &Bytes{data: make([]byte, VolumeClusters*ClusterSize)}
}

// Bytes returns a copy of the image contents, safe for the caller to mutate.
func (b *Bytes) Bytes() []byte {
	out := make([]byte, len(b.data))
	copy(out, b.data)

	return out
}

// Section returns a writable source over the image, mirroring what
// device.Open yields for a file on disk.
func (b *Bytes) Section() *device.Section {
	return device.NewSection("memory.img", b, b)
}

func (b *Bytes) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off+int64(len(p)) > int64(len(b.data)) {
		return 0, io.EOF
	}

	return copy(p, b.data[off:]), nil
}

func (b *Bytes) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 || off+int64(len(p)) > int64(len(b.data)) {
		return 0, io.EOF
	}

	return copy(b.data[off:], p), nil
}

func (b *Bytes) writeBootSector() {
	boot := b.data[:ntfs.SectorSize]

	copy(boot[3:11], ntfs.OemSignature)
	binary.LittleEndian.PutUint16(boot[11:13], BytesPerSector)
	boot[13] = SectorsPerCluster
	binary.LittleEndian.PutUint32(boot[0x28:0x2C], VolumeClusters*SectorsPerCluster)

	binary.LittleEndian.PutUint64(boot[0x30:0x38], MFTCluster)
	binary.LittleEndian.PutUint64(boot[0x38:0x40], MirrorCluster)

	// A negative run length means the record size is 2 raised to its magnitude:
	// -10 is the encoding for the 1024-byte records NTFS has always used.
	boot[0x40] = 0xF6
	boot[0x44] = SectorsPerCluster
}

func (b *Bytes) writeRecord(index int64, record []byte) {
	offset := int64(MFTCluster)*ClusterSize + index*ntfs.RecordSize
	copy(b.data[offset:offset+ntfs.RecordSize], record)
}

// writeMirrorPayload copies the first mirrored records of $mft into the extent
// that $mftmirr describes.
func (b *Bytes) writeMirrorPayload() {
	base := int64(MirrorCluster * ClusterSize)

	for index := range int64(ntfs.MirrorRecordCount) {
		from := int64(MFTCluster*ClusterSize) + index*ntfs.RecordSize
		to := base + index*ntfs.RecordSize
		copy(b.data[to:to+ntfs.RecordSize], b.data[from:from+ntfs.RecordSize])
	}
}

// newRecord builds a minimal in-use MFT record carrying no attributes.
func (b *Bytes) newRecord(flags ntfs.RecordFlags) []byte {
	record := make([]byte, ntfs.RecordSize)

	copy(record[0:4], ntfs.RecordMagic)
	binary.LittleEndian.PutUint16(record[4:6], 0x30)    // fixup offset
	binary.LittleEndian.PutUint16(record[6:8], 1)       // fixup count
	binary.LittleEndian.PutUint16(record[0x10:0x12], 1) // sequence number
	binary.LittleEndian.PutUint16(record[0x12:0x14], 1) // hard link count
	binary.LittleEndian.PutUint16(record[0x14:0x16], attributesAt)
	binary.LittleEndian.PutUint16(record[0x16:0x18], uint16(flags))
	binary.LittleEndian.PutUint32(record[0x18:0x1C], uint32(ntfs.RecordSize))
	binary.LittleEndian.PutUint32(record[0x1C:0x20], uint32(ntfs.RecordSize))

	return record
}

// newDataRecord builds a record whose single non-resident $data attribute
// describes one run of length clusters starting at firstCluster.
func (b *Bytes) newDataRecord(length uint8, firstCluster uint8, contentSize int64) []byte {
	record := b.newRecord(ntfs.FlagInUse)
	attr := record[attributesAt:]

	binary.LittleEndian.PutUint32(attr[0:4], uint32(ntfs.TypeData))
	binary.LittleEndian.PutUint32(attr[4:8], nonResidentHeaderSize+4)
	attr[8] = 1 // non-resident
	binary.LittleEndian.PutUint16(attr[0x20:0x22], runlistAt)
	binary.LittleEndian.PutUint64(attr[0x28:0x30], uint64(int64(length)*ClusterSize))
	binary.LittleEndian.PutUint64(attr[0x38:0x40], uint64(contentSize))

	// One run of length clusters at firstCluster: header 0x21 declares a
	// one-byte length followed by a one-byte cluster delta.
	attr[runlistAt] = 0x21
	attr[runlistAt+1] = length
	attr[runlistAt+2] = firstCluster

	return record
}

// newMirrorRecord builds the $mftmirr record: a header whose $data attribute
// points at the contiguous payload holding the mirrored copies.
func (b *Bytes) newMirrorRecord() []byte {
	return b.newDataRecord(1, MirrorCluster, int64(ntfs.MirrorRecordCount)*ntfs.RecordSize)
}
