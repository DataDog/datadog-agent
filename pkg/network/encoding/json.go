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

	handleZeroValues(conns)
	return conns, nil
}

func (j jsonSerializer) ContentType() string {
	return ContentTypeJSON
}

var _ Marshaler = jsonSerializer{}
var _ Unmarshaler = jsonSerializer{}

// this code is a hack to fix the way zero value maps are handled during a
// roundtrip (eg. marshaling/unmarshaling) by the JSON marshaler as we use
// the `EmitDefaults` option.  please note this function is executed *only for
// debugging purposes* since the JSON marshaller is not used for communication
// between system-probe and the process-agent.
// TODO: Make this more future-proof using reflection (we don't care about they
// perfomance penalty of doing so because this only runs during tests)
func handleZeroValues(conns *model.Connections) {
	if conns == nil {
		return
	}

	if len(conns.CompilationTelemetryByAsset) == 0 {
		conns.CompilationTelemetryByAsset = nil
	}

	for _, c := range conns.Conns {
		if len(c.DnsCountByRcode) == 0 {
			c.DnsCountByRcode = nil
		}
		if len(c.DnsStatsByDomain) == 0 {
			c.DnsStatsByDomain = nil
		}
		if len(c.DnsStatsByDomainByQueryType) == 0 {
			c.DnsStatsByDomainByQueryType = nil
		}
	}
}
