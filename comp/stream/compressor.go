package stream

type Compressor interface {
	AddItem(data []byte) error
	Close() ([]byte, error)
}
