package request

import (
	"github.com/gogo/protobuf/jsonpb"
	"strings"

	model "github.com/DataDog/agent-payload/process"
)

var (
	pSerializer = protoSerializer{}
	jSerializer = jsonSerializer{
		marshaler: jsonpb.Marshaler{
			EmitDefaults: true,
		},
	}
)

// Marshaler is an interface implemented by all process request serializers
type Marshaler interface {
	Marshal(r *model.ProcessRequest) ([]byte, error)
	ContentType() string
}

// Unmarshaler is an interface implemented by all process request deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*model.ProcessRequest, error)
}

// GetMarshaler returns the appropriate StatsMarshaler based on the given accept header
func GetMarshaler(accept string) Marshaler {
	if strings.Contains(accept, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

// GetUnmarshaler returns the appropriate StatsUnmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}
