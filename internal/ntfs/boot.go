// Package ntfs holds the on-disk NTFS format: plain data plus the pure decoding
// that turns sector bytes into it. Nothing in this package performs I/O, which
// keeps every structure testable against a byte slice.
package ntfs

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// SectorSize is the fixed size of an NTFS sector and boot sector.
const SectorSize = 512

// RecordSize is the size of one MFT record. NTFS encodes the real value in the
// boot sector; this is the value every volume created since NTFS 3.1 uses.
const RecordSize = 1024

// OemSignature marks a volume laid out by NTFS.
const OemSignature = "NTFS    "

// ErrNotNTFS reports bytes that are not an NTFS boot sector.
var ErrNotNTFS = errors.New("ntfs: volume does not carry an ntfs boot sector")

// BootSector is the decoded NTFS boot sector at the start of a volume.
//
// The boot sector does not describe where the $mft lives as a runlist: it gives
// the $mft zone cluster, and the real runlist has to be read out of record 0.
type BootSector struct {
	// OemSignature is the eight-byte factory signature, always "NTFS    ".
	OemSignature string
	// BytesPerSector is the sector size in bytes.
	BytesPerSector uint16
	// SectorsPerCluster is the cluster size expressed in sectors.
	SectorsPerCluster uint8
	// TotalSectors is the volume size in sectors, zero when unknown.
	TotalSectors uint64
	// MFTCluster is the $mft zone cluster: where record 0 lives.
	MFTCluster uint64
	// MFTMirrorCluster is the cluster holding the $mftmirr copy of record 0.
	MFTMirrorCluster uint64
	// MFTRunLength is the clusters-per-MFT-record field. A negative value means
	// the record size is 2 raised to its absolute value.
	MFTRunLength int8
	// IndexRunLength is the clusters-per-index-record field, same encoding.
	IndexRunLength int8
}

// ClusterSize reports the cluster size in bytes.
func (b BootSector) ClusterSize() int64 {
	return int64(b.BytesPerSector) * int64(b.SectorsPerCluster)
}

// MFTRecordSize reports the size of one MFT record in bytes.
func (b BootSector) MFTRecordSize() int64 {
	return runLengthToSize(b.MFTRunLength, b.ClusterSize())
}

// IndexRecordSize reports the size of one index record in bytes.
func (b BootSector) IndexRecordSize() int64 {
	return runLengthToSize(b.IndexRunLength, b.ClusterSize())
}

// runLengthToSize decodes the signed clusters-per-record encoding NTFS uses in
// the boot sector.
func runLengthToSize(n int8, clusterSize int64) int64 {
	if n < 0 {
		// -10 means 2^10 bytes, which decouples the record size from the
		// cluster size; anything above 20 is nonsense.
		shift := -int(n)
		if shift > 20 {
			return 0
		}

		return 1 << shift
	}

	return int64(n) * clusterSize
}

// ParseBootSector decodes the NTFS boot sector found at the start of a volume.
func ParseBootSector(b []byte) (BootSector, error) {
	if len(b) < SectorSize {
		return BootSector{}, fmt.Errorf("ntfs: boot sector truncated at %d bytes", len(b))
	}

	bs := BootSector{
		OemSignature:      string(b[3:11]),
		BytesPerSector:    binary.LittleEndian.Uint16(b[11:13]),
		SectorsPerCluster: b[13],
		TotalSectors:      binary.LittleEndian.Uint64(b[0x28:0x30]),
		MFTCluster:        binary.LittleEndian.Uint64(b[0x30:0x38]),
		MFTMirrorCluster:  binary.LittleEndian.Uint64(b[0x38:0x40]),
		MFTRunLength:      int8(b[0x40]),
		IndexRunLength:    int8(b[0x44]),
	}

	if bs.OemSignature != OemSignature {
		return bs, fmt.Errorf("%w: signature %q", ErrNotNTFS, bs.OemSignature)
	}

	if bs.BytesPerSector == 0 || bs.SectorsPerCluster == 0 {
		return bs, errors.New("ntfs: boot sector declares a zero cluster geometry")
	}

	if bs.MFTRecordSize() <= 0 {
		return bs, fmt.Errorf("ntfs: boot sector declares an unusable mft record size %d", bs.MFTRecordSize())
	}

	return bs, nil
}

// Run is one decoded runlist entry: a contiguous span of clusters that begins
// at FirstCluster and covers Length clusters.
type Run struct {
	FirstCluster uint64
	Length       uint64
}

// ClusterCount reports the total number of clusters the runs cover.
func ClusterCount(runs []Run) uint64 {
	var total uint64
	for _, run := range runs {
		total += run.Length
	}

	return total
}

// ClusterAt resolves a runlist-relative cluster index to a volume cluster.
// It reports false when the index falls past the end of the runlist.
func ClusterAt(runs []Run, index int64) (uint64, bool) {
	if index < 0 {
		return 0, false
	}

	for _, run := range runs {
		if index < int64(run.Length) {
			return run.FirstCluster + uint64(index), true
		}

		index -= int64(run.Length)
	}

	return 0, false
}

// ParseRunList decodes an NTFS runlist. baseCluster, when non-zero, overrides
// the first cluster of the first run; pass zero to keep the decoded value.
func ParseRunList(b []byte, baseCluster uint64) ([]Run, int64, error) {
	var (
		runs     []Run
		cursor   int64
		size     int64
		cursorAt int
	)

	for cursorAt < len(b) {
		header := b[cursorAt]
		if header == 0 {
			break
		}
		cursorAt++

		lengthBytes := int(header & 0x0F)
		offsetBytes := int(header >> 4)
		if lengthBytes == 0 || offsetBytes == 0 {
			return nil, 0, errors.New("ntfs: runlist header has a zero field width")
		}
		if cursorAt+lengthBytes+offsetBytes > len(b) {
			return nil, 0, errors.New("ntfs: runlist entry truncated")
		}

		length, err := readField(b[cursorAt : cursorAt+lengthBytes])
		if err != nil {
			return nil, 0, fmt.Errorf("ntfs: reading runlist length: %w", err)
		}
		cursorAt += lengthBytes

		delta, err := readField(b[cursorAt : cursorAt+offsetBytes])
		if err != nil {
			return nil, 0, fmt.Errorf("ntfs: reading runlist offset: %w", err)
		}
		cursorAt += offsetBytes

		cursor += signedField(delta, offsetBytes)
		runs = append(runs, Run{FirstCluster: uint64(max(cursor, 0)), Length: length})
		size += int64(length)
	}

	if len(runs) == 0 {
		return nil, 0, ErrNoRuns
	}

	if baseCluster != 0 {
		runs[0].FirstCluster = baseCluster
	}

	return runs, size, nil
}

// ErrNoRuns reports a runlist that decoded to no runs at all.
var ErrNoRuns = errors.New("ntfs: empty runlist")

// readField decodes a little-endian runlist field of 1, 2, 4 or 8 bytes.
func readField(b []byte) (uint64, error) {
	switch len(b) {
	case 1:
		return uint64(b[0]), nil
	case 2:
		return uint64(binary.LittleEndian.Uint16(b)), nil
	case 4:
		return uint64(binary.LittleEndian.Uint32(b)), nil
	case 8:
		return binary.LittleEndian.Uint64(b), nil
	default:
		return 0, fmt.Errorf("ntfs: unsupported runlist field width of %d bytes", len(b))
	}
}

// signedField reinterprets an unsigned runlist field as a two's complement
// value of the given width; runlist deltas may be negative.
func signedField(v uint64, width int) int64 {
	switch width {
	case 1:
		return int64(int8(v))
	case 2:
		return int64(int16(v))
	case 4:
		return int64(int32(v))
	default:
		return int64(v)
	}
}
