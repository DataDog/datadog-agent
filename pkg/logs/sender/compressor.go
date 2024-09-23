package sender

import (
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
)

// Compressor wraps the compression component.
// (TODO: This may not be needed)
type Compressor struct {
	compression compression.Component
}

// NewCompressor creates a new Compressor.
func NewCompressor(compression compression.Component) *Compressor {
	return &Compressor{
		compression: compression,
	}
}

func (c *Compressor) name() string {
	return c.compression.ContentEncoding()
}

func (c *Compressor) encode(payload []byte) ([]byte, error) {
	return c.compression.Compress(payload)
}
