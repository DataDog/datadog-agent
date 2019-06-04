package encoding

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	agent "github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/gogo/protobuf/jsonpb"
)

// ContentTypeJSON holds the HTML content-type of a JSON payload
const ContentTypeJSON = "application/json"

type jsonSerializer struct {
	marshaller jsonpb.Marshaler
}

func (j jsonSerializer) Marshal(conns *ebpf.Connections) ([]byte, error) {
	var (
		agentConns = make([]*agent.Connection, len(conns.Conns))
		addrCache  = make(AddrCache)
	)
	for i, conn := range conns.Conns {
		agentConns[i] = FormatConnection(conn, addrCache)
	}
	payload := &agent.Connections{Conns: agentConns}
	writer := new(bytes.Buffer)
	err := j.marshaller.Marshal(writer, payload)
	return writer.Bytes(), err
}

func (jsonSerializer) Unmarshal(blob []byte) (*agent.Connections, error) {
	conns := new(agent.Connections)
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
