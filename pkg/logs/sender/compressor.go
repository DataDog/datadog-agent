package sender

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type Compressor struct {
	content_encoding ContentEncoding
}

var (
	tlmTotalPayloads = telemetry.NewCounter("logscompressor", "total_payloads",
		nil, "Total payloads in the compressor serializer")
	tlmBytesIn = telemetry.NewCounter("logscompressor", "bytes_in",
		nil, "Count of bytes entering the compressor serializer")
	tlmBytesOut = telemetry.NewCounter("logscompressor", "bytes_out",
		nil, "Count of bytes out the compressor serializer")
)

func NewCompressor(content_encoding ContentEncoding) *Compressor {
	return &Compressor{
		content_encoding: content_encoding,
	}
}

func (c *Compressor) name() string {
	return c.content_encoding.name()
}

func (c *Compressor) encode(payload []byte) ([]byte, error) {
	uncompressedSize := len(payload)

	payload, error := c.content_encoding.encode(payload)
	if error != nil {
		return nil, error
	}

	compressedSize := len(payload)

	tlmTotalPayloads.Add(1)
	tlmBytesIn.Add(float64(uncompressedSize))
	tlmBytesOut.Add(float64(compressedSize))

	return payload, nil
}
