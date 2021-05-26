package request

import (
	"github.com/gogo/protobuf/proto"

	model "github.com/DataDog/agent-payload/process"
)

// ContentTypeProtobuf holds the HTML content-type of a Protobuf payload
const ContentTypeProtobuf = "application/protobuf"

type protoSerializer struct{}

// Marshal returns the proto encoding of the ProcessRequest
func (protoSerializer) Marshal(r *model.ProcessRequest) ([]byte, error) {
	buf, err := proto.Marshal(r)
	return buf, err
}

// Unmarshal parses the proto-encoded ProcessRequest
func (protoSerializer) Unmarshal(blob []byte) (*model.ProcessRequest, error) {
	req := new(model.ProcessRequest)
	if err := proto.Unmarshal(blob, req); err != nil {
		return nil, err
	}
	return req, nil
}

// ContentType returns ContentTypeProtobuf
func (p protoSerializer) ContentType() string {
	return ContentTypeProtobuf
}

var _ Marshaler = protoSerializer{}
var _ Unmarshaler = protoSerializer{}
