package volume_test

import (
	"errors"
	"io"
	"testing"

	"github.com/fabianoflorentino/ntfsck/internal/ntfs"
	"github.com/fabianoflorentino/ntfsck/internal/volume"
	"github.com/fabianoflorentino/ntfsck/internal/volumeimg"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	t.Run("exposes the boot sector geometry", func(t *testing.T) {
		t.Parallel()

		vol := open(t, volumeimg.New())

		if got := vol.Boot().OemSignature; got != ntfs.OemSignature {
			t.Errorf("OemSignature = %q, want %q", got, ntfs.OemSignature)
		}

		if got := vol.ClusterSize(); got != volumeimg.ClusterSize {
			t.Errorf("ClusterSize() = %d, want %d", got, volumeimg.ClusterSize)
		}

		if got := vol.RecordCount(); got != volumeimg.RecordCount {
			t.Errorf("RecordCount() = %d, want %d", got, volumeimg.RecordCount)
		}
	})

	t.Run("rejects a non-ntfs volume", func(t *testing.T) {
		t.Parallel()

		_, err := volume.Open(volumeimg.NewEmpty().Section())
		if !errors.Is(err, ntfs.ErrNotNTFS) {
			t.Errorf("Open() error = %v, want ntfs.ErrNotNTFS", err)
		}
	})
}

func TestMFTRecord(t *testing.T) {
	t.Parallel()

	vol := open(t, volumeimg.New())

	t.Run("decodes a system record", func(t *testing.T) {
		t.Parallel()

		header, err := vol.MFTRecord(ntfs.MFT)
		if err != nil {
			t.Fatalf("MFTRecord() error = %v", err)
		}

		if !header.IsInUse() {
			t.Error("IsInUse() = false, want true")
		}
	})

	t.Run("rejects an out-of-range index", func(t *testing.T) {
		t.Parallel()

		if _, err := vol.MFTRecord(vol.RecordCount()); !errors.Is(err, volume.ErrRecordOutOfRange) {
			t.Errorf("MFTRecord() error = %v, want ErrRecordOutOfRange", err)
		}

		if _, err := vol.MFTRecord(-1); !errors.Is(err, volume.ErrRecordOutOfRange) {
			t.Errorf("MFTRecord() error = %v, want ErrRecordOutOfRange", err)
		}
	})
}

func TestReadAt(t *testing.T) {
	t.Parallel()

	vol := open(t, volumeimg.New())

	t.Run("reads the oem signature inside the boot sector", func(t *testing.T) {
		t.Parallel()

		buf := make([]byte, len(ntfs.OemSignature))
		if err := vol.ReadAt(buf, 3); err != nil {
			t.Fatalf("ReadAt() error = %v", err)
		}

		if string(buf) != ntfs.OemSignature {
			t.Errorf("ReadAt() = %q, want %q", buf, ntfs.OemSignature)
		}
	})

	t.Run("wraps a read past the end of the medium", func(t *testing.T) {
		t.Parallel()

		err := vol.ReadAt(make([]byte, 4096), 1<<40)
		if !errors.Is(err, io.EOF) {
			t.Errorf("ReadAt() error = %v, want io.EOF", err)
		}
	})
}

func open(t *testing.T, img *volumeimg.Bytes) *volume.Volume {
	t.Helper()

	vol, err := volume.Open(img.Section())
	if err != nil {
		t.Fatalf("volume.Open() error = %v", err)
	}

	t.Cleanup(func() {
		if err := vol.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return vol
}
