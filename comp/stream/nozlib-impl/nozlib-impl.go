package streamimpl

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/comp/stream"
)

type nozlibStreamComponent struct {
}

var _ stream.Component = (*nozlibStreamComponent)(nil)

func NewStream() stream.Component {
	return &nozlibStreamComponent{}
}

func (sc *nozlibStreamComponent) NewCompressor(input, output *bytes.Buffer,
	maxPayloadSize, maxUncompressedSize int,
	header, footer []byte,
	separator []byte) (stream.Compressor, error) {

	return nil, errors.New("not implemented")
}

func (sc *nozlibStreamComponent) NewJSONPayloadBuilder(shareAndLockBuffers bool) stream.JSONPayloadBuilder {
	return &noneJSONPayloadBuilder{}
}

func (sc *nozlibStreamComponent) IsAvailable() bool {
	return false
}
