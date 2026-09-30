// Package volume opens an NTFS volume over a Source and owns the translation
// between cluster addresses and flat byte offsets. It is the boundary between
// pure format decoding in package ntfs and any real medium.
package volume

import (
	"errors"
	"fmt"

	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
)

// ErrRecordOutOfRange reports a record index beyond the $MFT extent.
var ErrRecordOutOfRange = errors.New("volume: mft record index out of range")

// ErrNoMFTExtent reports a volume whose record 0 does not describe an extent.
var ErrNoMFTExtent = errors.New("volume: mft record 0 carries no non-resident data attribute")

// Volume is an open NTFS volume: the source plus the geometry decoded from its
// boot sector and from the $mft record 0.
type Volume struct {
	source  Source
	boot    ntfs.BootSector
	mftRuns []ntfs.Run
}

// Open reads and validates the boot sector of source, then resolves the $mft
// extent from record 0. Ownership of source moves to the returned Volume,
// which releases it on Close.
func Open(source Source) (*Volume, error) {
	sector := make([]byte, ntfs.SectorSize)
	if _, err := source.ReadAt(sector, 0); err != nil {
		return nil, fmt.Errorf("volume: reading boot sector: %w", err)
	}

	boot, err := ntfs.ParseBootSector(sector)
	if err != nil {
		return nil, fmt.Errorf("volume: %s: %w", source.Name(), err)
	}

	vol := &Volume{source: source, boot: boot}

	// Record 0 sits at the $mft zone cluster the boot sector points at, and its
	// $data runlist is the only authoritative description of the $mft extent.
	head := make([]byte, boot.MFTRecordSize())
	offset := int64(boot.MFTCluster) * boot.ClusterSize()
	if _, err := source.ReadAt(head, offset); err != nil {
		return nil, fmt.Errorf("volume: reading mft record 0 at offset %d: %w", offset, err)
	}

	runs, _, err := ntfs.DataRuns(head)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoMFTExtent, err)
	}

	vol.mftRuns = runs

	return vol, nil
}

// Boot returns the decoded boot sector.
func (v *Volume) Boot() ntfs.BootSector { return v.boot }

// ClusterSize reports the cluster size in bytes.
func (v *Volume) ClusterSize() int64 { return v.boot.ClusterSize() }

// RecordSize reports the size of one MFT record in bytes.
func (v *Volume) RecordSize() int64 { return v.boot.MFTRecordSize() }

// MFTRuns returns the extent of the $mft as decoded from record 0.
func (v *Volume) MFTRuns() []ntfs.Run { return v.mftRuns }

// RecordCount reports how many MFT records the $mft extent holds.
func (v *Volume) RecordCount() int64 {
	bytes := int64(ntfs.ClusterCount(v.mftRuns)) * v.ClusterSize()
	if size := v.RecordSize(); size > 0 {
		return bytes / size
	}

	return 0
}

// MFTRecordBytes reads the raw bytes of MFT record index. The caller owns the
// returned slice.
func (v *Volume) MFTRecordBytes(index int64) ([]byte, error) {
	recordSize := v.RecordSize()
	if recordSize <= 0 {
		return nil, errors.New("volume: boot sector declares a zero mft record size")
	}

	if index < 0 || index >= v.RecordCount() {
		return nil, fmt.Errorf("%w: %d not in [0, %d)", ErrRecordOutOfRange, index, v.RecordCount())
	}

	offset, err := v.MFTOffset(index)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, recordSize)
	if _, err := v.source.ReadAt(buf, offset); err != nil {
		return nil, fmt.Errorf("volume: reading mft record %d at offset %d: %w", index, offset, err)
	}

	return buf, nil
}

// MFTOffset returns the byte offset of MFT record index, resolving the record
// through the $mft runlist so a fragmented $mft reads correctly.
func (v *Volume) MFTOffset(index int64) (int64, error) {
	clusterSize := v.ClusterSize()
	recordSize := v.RecordSize()

	relative := index * recordSize
	cluster, ok := ntfs.ClusterAt(v.mftRuns, relative/clusterSize)
	if !ok {
		return 0, fmt.Errorf("%w: %d not in [0, %d)", ErrRecordOutOfRange, index, v.RecordCount())
	}

	return int64(cluster)*clusterSize + relative%clusterSize, nil
}

// MFTRecord reads and decodes the header of MFT record index.
func (v *Volume) MFTRecord(index int64) (ntfs.RecordHeader, error) {
	raw, err := v.MFTRecordBytes(index)
	if err != nil {
		return ntfs.RecordHeader{}, err
	}

	header, err := ntfs.DecodeRecordHeader(raw)
	if err != nil {
		return ntfs.RecordHeader{}, fmt.Errorf("volume: mft record %d: %w", index, err)
	}

	return header, nil
}

// ReadAt reads p from the volume at a byte offset. NTFS structures are not all
// cluster aligned, so this method imposes no alignment of its own.
func (v *Volume) ReadAt(p []byte, offset int64) error {
	if _, err := v.source.ReadAt(p, offset); err != nil {
		return fmt.Errorf("volume: reading %d bytes at offset %d: %w", len(p), offset, err)
	}

	return nil
}

// Source exposes the underlying port so the repair engine can write through the
// same handle it validated.
func (v *Volume) Source() Source { return v.source }

// Close releases the underlying source.
func (v *Volume) Close() error {
	if err := v.source.Close(); err != nil {
		return fmt.Errorf("volume: closing %s: %w", v.source.Name(), err)
	}

	return nil
}
