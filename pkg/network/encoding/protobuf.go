package encoding

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/gogo/protobuf/proto"
)

// ContentTypeProtobuf holds the HTML content-type of a Protobuf payload
const ContentTypeProtobuf = "application/protobuf"

type protoSerializer struct{}

func (protoSerializer) Marshal(conns *network.Connections) ([]byte, error) {
	payload := modelConnections(conns)
	buf, err := proto.Marshal(payload)
	returnToPool(payload)
	return buf, err
}

func (protoSerializer) Unmarshal(blob []byte) (*model.Connections, error) {
	conns := new(model.Connections)
	if err := proto.Unmarshal(blob, conns); err != nil {
		return nil, err
	}
	return conns, nil
}

func (p protoSerializer) ContentType() string {
	return ContentTypeProtobuf
}

var _ Marshaler = protoSerializer{}
var _ Unmarshaler = protoSerializer{}
