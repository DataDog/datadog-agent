package request

import (
	"strings"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
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
	Marshal(r *pbgo.ProcessStatRequest) ([]byte, error)
	ContentType() string
}

// Unmarshaler is an interface implemented by all process request deserializers
type Unmarshaler interface {
	Unmarshal([]byte) (*pbgo.ProcessStatRequest, error)
}

// GetMarshaler returns the appropriate Marshaler based on the given accept header
func GetMarshaler(accept string) Marshaler {
	if strings.Contains(accept, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}

// GetUnmarshaler returns the appropriate Unmarshaler based on the given content type
func GetUnmarshaler(ctype string) Unmarshaler {
	if strings.Contains(ctype, ContentTypeProtobuf) {
		return pSerializer
	}

	return jSerializer
}
