package util

type NamedPipe interface {
	Open() error
	Ready() bool
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	Close() error
}
