package request

import (
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

// ContentTypeProtobuf holds the HTML content-type of a Protobuf payload
const ContentTypeProtobuf = "application/protobuf"

type protoSerializer struct{}

// Marshal returns the proto encoding of the ProcessStatRequest
func (protoSerializer) Marshal(r *pbgo.ProcessStatRequest) ([]byte, error) {
	buf, err := proto.Marshal(r)
	return buf, err
}

// Unmarshal parses the proto-encoded ProcessStatRequest
func (protoSerializer) Unmarshal(blob []byte) (*pbgo.ProcessStatRequest, error) {
	req := new(pbgo.ProcessStatRequest)
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
