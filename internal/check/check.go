package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
)

// ErrUnreadableStructure reports a structure ntfsck cannot judge. A structure
// that cannot be read is never reported as clean.
var ErrUnreadableStructure = errors.New("check: structure cannot be read")

// Volume is the read-only port the checker needs, declared at the consumer so
// the use case depends on the narrowest contract that still carries the work.
type Volume interface {
	Boot() ntfs.BootSector
	RecordCount() int64
	MFTRecord(index int64) (ntfs.RecordHeader, error)
	MFTRecordBytes(index int64) ([]byte, error)
	ReadAt(p []byte, offset int64) error
}

// Checker verifies NTFS metadata consistency. It never writes.
type Checker struct {
	volume Volume
}

// New returns a Checker over vol.
func New(vol Volume) *Checker {
	return &Checker{volume: vol}
}

// Run inspects every structure the checker currently understands. Structures it
// cannot judge yet are reported rather than omitted, so the output never implies
// more coverage than exists.
func (c *Checker) Run(ctx context.Context) (Report, error) {
	report := Report{
		Volume:      c.volume.Boot().OemSignature,
		ClusterSize: c.volume.Boot().ClusterSize(),
		RecordCount: c.volume.RecordCount(),
	}

	findings, err := c.checkMirror(ctx)
	if err != nil {
		return report, err
	}
	report.Findings = findings

	report.NotCovered = []string{
		"$bitmap",
		"$logfile",
		"$secure",
	}

	return report, nil
}

// checkMirror compares the leading records of $mft against the copy of the same
// records kept in $mftmirr. This divergence is what motivated the project.
func (c *Checker) checkMirror(ctx context.Context) ([]Finding, error) {
	header, err := c.volume.MFTRecord(0)
	if err != nil {
		return nil, fmt.Errorf("check: reading $mft record 0: %w", err)
	}
	if !header.IsInUse() {
		return nil, fmt.Errorf("%w: $mft record 0 is not in use", ErrUnreadableStructure)
	}
	mirror, err := c.readMirrorExtent()
	if err != nil {
		//nolint:nilerr // an unreadable $mftmirr is the finding itself, not a failure to run the check.
		return []Finding{{
			Structure: "$mftmirr",
			Severity:  SeverityError,
			Detail:    err.Error(),
		}}, nil
	}

	findings := make([]Finding, 0, ntfs.MirrorRecordCount+1)
	diverged := 0

	for index := range int64(ntfs.MirrorRecordCount) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("check: %w", err)
		}

		authoritative, err := c.volume.MFTRecordBytes(index)
		if err != nil {
			return nil, fmt.Errorf("check: reading $mft record %d: %w", index, err)
		}

		mirrored, err := mirror.record(index)
		if err != nil {
			findings = append(findings, Finding{
				Structure: "$mftmirr",
				Severity:  SeverityError,
				Detail:    fmt.Sprintf("record %d unreadable: %v", index, err),
			})
			continue
		}

		if bytes.Equal(authoritative, mirrored) {
			findings = append(findings, Finding{
				Structure: "$mftmirr",
				Severity:  SeverityOK,
				Detail:    fmt.Sprintf("record %d matches $mft", index),
			})
			continue
		}

		diverged++
		findings = append(findings, Finding{
			Structure: "$mftmirr",
			Severity:  SeverityWarn,
			Detail:    fmt.Sprintf("record %d (%s) differs from $mft", index, nameOf(index)),
		})
	}

	if diverged == 0 {
		return append(findings, Finding{
			Structure: "$mftmirr",
			Severity:  SeverityOK,
			Detail:    "every mirrored record matches $mft",
		}), nil
	}

	return append(findings, Finding{
		Structure: "$mftmirr",
		Severity:  SeverityWarn,
		Detail: fmt.Sprintf(
			"%d of %d mirrored records diverge from $mft",
			diverged, ntfs.MirrorRecordCount,
		),
	}), nil
}

// mirrorExtent is the in-memory copy of the $mftmirr payload.
type mirrorExtent struct {
	bytes []byte
}

// readMirroredRecord returns one mirrored record from the extent.
func (m mirrorExtent) record(index int64) ([]byte, error) {
	if m.bytes == nil {
		return nil, errors.New("$mftmirr payload was not read")
	}

	start := index * ntfs.RecordSize
	if start < 0 || start+ntfs.RecordSize > int64(len(m.bytes)) {
		return nil, fmt.Errorf(
			"mirrored extent holds %d bytes, record %d needs %d",
			len(m.bytes), index, start+ntfs.RecordSize,
		)
	}

	return m.bytes[start : start+ntfs.RecordSize], nil
}

// readMirrorExtent locates the $mftmirr payload through its own $data runlist
// and reads it into memory.
func (c *Checker) readMirrorExtent() (mirrorExtent, error) {
	raw, err := c.volume.MFTRecordBytes(ntfs.MFTMirror)
	if err != nil {
		return mirrorExtent{}, fmt.Errorf("reading $mftmirr record: %w", err)
	}

	runs, contentSize, err := ntfs.DataRuns(raw)
	if err != nil {
		return mirrorExtent{}, fmt.Errorf("locating $mftmirr payload: %w", err)
	}

	clusterSize := c.volume.Boot().ClusterSize()
	if clusterSize <= 0 {
		return mirrorExtent{}, errors.New("boot sector declares a zero cluster size")
	}

	// The mirrored payload is a contiguous copy, so only the first run matters.
	extent := runs[0]
	available := int64(extent.Length) * clusterSize
	size := min(contentSize, available)

	offset := int64(extent.FirstCluster) * clusterSize
	buf := make([]byte, size)
	if err := c.volume.ReadAt(buf, offset); err != nil {
		return mirrorExtent{}, fmt.Errorf("reading $mftmirr payload at offset %d: %w", offset, err)
	}

	return mirrorExtent{bytes: buf}, nil
}

func nameOf(index int64) string {
	if name := ntfs.SystemName(uint64(index)); name != "" {
		return name
	}

	return fmt.Sprintf("record %d", index)
}
