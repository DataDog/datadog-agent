package streamimpl

import (
	"bytes"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/stream"
)

type streamComponent struct {
	config config.Component
}

var _ stream.Component = (*streamComponent)(nil)

func NewStream(config config.Component) stream.Component {
	return &streamComponent{
		config: config,
	}
}

func (sc *streamComponent) NewCompressor(input, output *bytes.Buffer,
	maxPayloadSize, maxUncompressedSize int,
	header, footer []byte,
	separator []byte) (stream.Compressor, error) {

	return NewCompressor(input, output, maxPayloadSize, maxUncompressedSize, header, footer, separator)
}

func (sc *streamComponent) NewJSONPayloadBuilder(shareAndLockBuffers bool) stream.JSONPayloadBuilder {
	return NewJSONPayloadBuilder(shareAndLockBuffers, sc.config)
}

func (sc *streamComponent) IsAvailable() bool {
	return true
}
