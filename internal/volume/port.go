package volume

import "io"

// Source is the port through which the domain reaches raw volume bytes. It is
// declared here, at the consumer, so the checker and the repair engine never
// import an adapter package. Both a host block device and an in-memory image
// satisfy it.
type Source interface {
	io.ReaderAt
	io.WriterAt

	// Name reports the path or label the bytes came from.
	Name() string

	// Close releases the underlying handle.
	Close() error
}
