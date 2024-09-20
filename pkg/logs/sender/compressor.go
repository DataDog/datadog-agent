package sender

import (
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
)

// TODO: This may not be needed.
type Compressor struct {
	compression compression.Component
}

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
