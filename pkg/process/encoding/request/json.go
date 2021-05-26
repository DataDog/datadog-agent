package request

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"

	model "github.com/DataDog/agent-payload/process"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaler jsonpb.Marshaler
}

// Marshal returns the json encoding of the ProcessRequest
func (j jsonSerializer) Marshal(r *model.ProcessRequest) ([]byte, error) {
	writer := new(bytes.Buffer)

	err := j.marshaler.Marshal(writer, r)
	return writer.Bytes(), err
}

// Unmarshal parses the JSON-encoded ProcessRequest
func (jsonSerializer) Unmarshal(blob []byte) (*model.ProcessRequest, error) {
	req := new(model.ProcessRequest)
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
