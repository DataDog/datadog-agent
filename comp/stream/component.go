package stream

import (
	"bytes"
)

type Component interface {
	NewCompressor(
		input, output *bytes.Buffer, maxPayloadSize, maxUncompressedSize int,
		header, footer, separator []byte) (Compressor, error)
	NewJSONPayloadBuilder(shareAndLockBuffers bool) JSONPayloadBuilder
	IsAvailable() bool
}
