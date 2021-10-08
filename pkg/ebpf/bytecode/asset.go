package bytecode

import (
	"io"
)

// AssetReader describes the combination of both io.Reader and io.ReaderAt
type AssetReader interface {
	io.Reader
	io.ReaderAt
	io.Closer
}
