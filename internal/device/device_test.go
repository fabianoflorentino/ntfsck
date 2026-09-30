package device_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabianoflorentino/ntfsck/internal/device"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	path := writeTempFile(t, []byte("ntfsck"))

	t.Run("read-only refuses writes", func(t *testing.T) {
		t.Parallel()

		dev, err := device.Open(path, true)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer func() { _ = dev.Close() }()

		if !dev.IsReadOnly() {
			t.Error("IsReadOnly() = false, want true")
		}

		if _, err := dev.WriteAt([]byte("x"), 0); !errors.Is(err, device.ErrReadOnly) {
			t.Errorf("WriteAt() error = %v, want ErrReadOnly", err)
		}

		// The bytes on disk must be untouched.
		if got := readTempFile(t, path); string(got) != "ntfsck" {
			t.Errorf("file content = %q, want %q", got, "ntfsck")
		}
	})

	t.Run("read-write accepts writes", func(t *testing.T) {
		t.Parallel()

		rw := writeTempFile(t, []byte("ntfsck"))

		dev, err := device.Open(rw, false)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer func() { _ = dev.Close() }()

		if _, err := dev.WriteAt([]byte("X"), 3); err != nil {
			t.Fatalf("WriteAt() error = %v", err)
		}

		if err := dev.Sync(); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}

		if got := readTempFile(t, rw); string(got) != "ntfXck" {
			t.Errorf("file content = %q, want %q", got, "ntfXck")
		}
	})

	t.Run("reports a missing path", func(t *testing.T) {
		t.Parallel()

		if _, err := device.Open(filepath.Join(t.TempDir(), "absent.img"), true); err == nil {
			t.Error("Open() error = nil, want an error for a missing file")
		}
	})
}

func TestFileSize(t *testing.T) {
	t.Parallel()

	dev, err := device.Open(writeTempFile(t, make([]byte, 8192)), true)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = dev.Close() }()

	size, err := dev.Size()
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}

	if size != 8192 {
		t.Errorf("Size() = %d, want 8192", size)
	}
}

func TestSection(t *testing.T) {
	t.Parallel()

	t.Run("without a writer is read-only", func(t *testing.T) {
		t.Parallel()

		src := device.NewSection("memory.img", newStub(), nil)

		if !src.IsReadOnly() {
			t.Error("IsReadOnly() = false, want true")
		}

		if _, err := src.WriteAt([]byte("x"), 0); !errors.Is(err, device.ErrReadOnly) {
			t.Errorf("WriteAt() error = %v, want ErrReadOnly", err)
		}
	})

	t.Run("with a writer accepts writes", func(t *testing.T) {
		t.Parallel()

		stub := newStub()
		src := device.NewSection("memory.img", stub, stub)

		if _, err := src.WriteAt([]byte("ntfsck"), 0); err != nil {
			t.Fatalf("WriteAt() error = %v", err)
		}

		got := make([]byte, 6)
		if _, err := src.ReadAt(got, 0); err != nil {
			t.Fatalf("ReadAt() error = %v", err)
		}

		if string(got) != "ntfsck" {
			t.Errorf("ReadAt() = %q, want %q", got, "ntfsck")
		}
	})
}

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "volume.img")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}

	return path
}

func readTempFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	return content
}

type stub struct {
	buf []byte
}

func newStub() *stub { return &stub{buf: make([]byte, 512)} }

func (s *stub) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(s.buf)) {
		return 0, os.ErrClosed
	}

	return copy(p, s.buf[off:]), nil
}

func (s *stub) WriteAt(p []byte, off int64) (int, error) {
	return copy(s.buf[off:], p), nil
}
