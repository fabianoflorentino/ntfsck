// Package device is the NTFS volume access adapter. It satisfies the volume
// source port with a host file or block device handle, and adds the read-only
// guarantee ntfsck relies on: a handle opened read-only refuses every write
// instead of reaching the medium.
package device

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrReadOnly reports a write attempted on a read-only handle.
var ErrReadOnly = errors.New("device: handle is read-only")

// File is a source backed by a block device or image file on the host.
type File struct {
	file     *os.File
	path     string
	readOnly bool
}

// Open returns a File for path. When readOnly is true, every WriteAt fails with
// ErrReadOnly without touching the medium.
func Open(path string, readOnly bool) (*File, error) {
	flag := os.O_RDONLY
	if !readOnly {
		flag = os.O_RDWR
	}

	//nolint:gosec // the path is the volume the operator asked ntfsck to open.
	file, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, fmt.Errorf("device: opening %s: %w", path, err)
	}

	return &File{file: file, path: path, readOnly: readOnly}, nil
}

// NewFile returns a File over an already-open handle. The caller keeps
// ownership of file and must not Close it through the returned value.
func NewFile(path string, file *os.File) *File {
	return &File{file: file, path: path}
}

func (f *File) ReadAt(p []byte, off int64) (int, error) { return f.file.ReadAt(p, off) }

func (f *File) WriteAt(p []byte, off int64) (int, error) {
	if f.readOnly {
		return 0, fmt.Errorf("device: writing %d bytes at offset %d: %w", len(p), off, ErrReadOnly)
	}

	return f.file.WriteAt(p, off)
}

// Sync flushes buffered writes to stable storage.
func (f *File) Sync() error { return f.file.Sync() }

// Size reports the medium size in bytes.
func (f *File) Size() (int64, error) {
	info, err := f.file.Stat()
	if err != nil {
		return 0, fmt.Errorf("device: stating %s: %w", f.path, err)
	}

	return info.Size(), nil
}

// Name reports the path the handle was opened from.
func (f *File) Name() string { return f.path }

// IsReadOnly reports whether writes are refused on this handle.
func (f *File) IsReadOnly() bool { return f.readOnly }

func (f *File) Close() error { return f.file.Close() }

// Section is an in-memory source over caller-owned buffers. It is the seam the
// test suite uses to run the checker against a synthetic volume.
type Section struct {
	name     string
	reader   io.ReaderAt
	writer   io.WriterAt
	readOnly bool
}

// NewSection returns a Section reading from reader. A nil writer marks the
// section read-only.
func NewSection(name string, reader io.ReaderAt, writer io.WriterAt) *Section {
	return &Section{name: name, reader: reader, writer: writer, readOnly: writer == nil}
}

func (s *Section) ReadAt(p []byte, off int64) (int, error) { return s.reader.ReadAt(p, off) }

func (s *Section) WriteAt(p []byte, off int64) (int, error) {
	if s.readOnly {
		return 0, fmt.Errorf("device: writing %d bytes at offset %d: %w", len(p), off, ErrReadOnly)
	}

	return s.writer.WriteAt(p, off)
}

// Sync is a no-op: caller-owned buffers have no separate durable medium.
func (s *Section) Sync() error { return nil }

// Size reports zero: a section has no intrinsic length, only its reader's.
func (s *Section) Size() (int64, error) { return 0, nil }

// Name reports the section label.
func (s *Section) Name() string { return s.name }

// IsReadOnly reports whether writes are refused on this section.
func (s *Section) IsReadOnly() bool { return s.readOnly }

// Close is a no-op: the section does not own the buffers behind it.
func (s *Section) Close() error { return nil }
