package http2

import (
	"bytes"
	"fmt"
	"golang.org/x/net/http2/hpack"
)

var (
	// MagicFrame http2 magic
	MagicFrame = []byte{
		0x50, 0x52, 0x49, 0x20, 0x2a, 0x20, 0x48, 0x54, 0x54, 0x50, 0x2f, 0x32, 0x2e, 0x30, 0x0d, 0x0a, 0x0d, 0x0a, 0x53, 0x4d, 0x0d, 0x0a, 0x0d, 0x0a,
	}
)

// NewHeadersFrameMessage creates a new HTTP2 data frame message with the given header fields.
func NewHeadersFrameMessage(headerFields []hpack.HeaderField) ([]byte, error) {
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)

	for _, value := range headerFields {
		if err := enc.WriteField(value); err != nil {
			return nil, fmt.Errorf("error encoding field: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// ComposeMessage concatenates the given byte slices into a single byte slice.
func ComposeMessage(slices ...[]byte) []byte {
	var result []byte

	for _, s := range slices {
		result = append(result, s...)
	}

	return result
}
