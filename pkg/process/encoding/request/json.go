package request

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaler jsonpb.Marshaler
}

// Marshal returns the json encoding of the ProcessStatRequest
func (j jsonSerializer) Marshal(r *pbgo.ProcessStatRequest) ([]byte, error) {
	writer := new(bytes.Buffer)

	err := j.marshaler.Marshal(writer, r)
	return writer.Bytes(), err
}

// Unmarshal parses the JSON-encoded ProcessStatRequest
func (jsonSerializer) Unmarshal(blob []byte) (*pbgo.ProcessStatRequest, error) {
	req := new(pbgo.ProcessStatRequest)
	reader := bytes.NewReader(blob)
	if err := jsonpb.Unmarshal(reader, req); err != nil {
		return nil, err
	}
	return req, nil
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

var _ Marshaler = jsonSerializer{}
var _ Unmarshaler = jsonSerializer{}
