package encoding

import (
	"bytes"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/gogo/protobuf/jsonpb"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaller jsonpb.Marshaler
}

func (j jsonSerializer) Marshal(conns *network.Connections) ([]byte, error) {
	payload := modelConnections(conns)
	writer := new(bytes.Buffer)
	err := j.marshaller.Marshal(writer, payload)
	returnToPool(payload)
	return writer.Bytes(), err
}

func (jsonSerializer) Unmarshal(blob []byte) (*model.Connections, error) {
	conns := new(model.Connections)
	reader := bytes.NewReader(blob)
	if err := jsonpb.Unmarshal(reader, conns); err != nil {
		return nil, err
	}
	return conns, nil
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

var _ Marshaler = jsonSerializer{}
var _ Unmarshaler = jsonSerializer{}
