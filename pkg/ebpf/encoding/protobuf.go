package encoding

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	agent "github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/gogo/protobuf/proto"
)

// ContentTypeProtobuf holds the HTML content-type of a Protobuf payload
const ContentTypeProtobuf = "application/protobuf"

type protoSerializer struct{}

func (protoSerializer) Marshal(conns *ebpf.Connections) ([]byte, error) {
	var (
		agentConns = make([]*agent.Connection, len(conns.Conns))
		addrCache  = make(AddrCache)
	)
	for i, conn := range conns.Conns {
		agentConns[i] = FormatConnection(conn, addrCache)
	}
	payload := &agent.Connections{Conns: agentConns}
	return proto.Marshal(payload)
}

func (protoSerializer) Unmarshal(blob []byte) (*agent.Connections, error) {
	conns := new(agent.Connections)
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
