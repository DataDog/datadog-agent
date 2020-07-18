package traces

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func TestLazySpanUnmarshal(t *testing.T) {
	span := newTestSpan()
	marshaled, err := proto.Marshal(&span)
	require.NoError(t, err)

	lazy, err := NewLazySpan(marshaled)
	require.NoError(t, err)

	require.Equal(t, lazy.TraceID(), span.TraceID)
	require.Equal(t, lazy.SpanID(), span.SpanID)
	require.Equal(t, lazy.ParentID(), span.ParentID)
	require.Equal(t, lazy.UnsafeType(), span.Type)
	require.Equal(t, lazy.UnsafeService(), span.Service)
	require.Equal(t, lazy.UnsafeName(), span.Name)
	require.Equal(t, lazy.UnsafeResource(), span.Resource)
	require.Equal(t, lazy.Start(), span.Start)
	require.Equal(t, lazy.Duration(), span.Duration)
}

func TestLazySpanMutate(t *testing.T) {
	span := newTestSpan()
	marshaled, err := proto.Marshal(&span)
	require.NoError(t, err)

	lazy, err := NewLazySpan(marshaled)
	require.NoError(t, err)

	lazy.SetType("new_type")
	lazy.SetService("new_service")
	lazy.SetName("new_name")
	lazy.SetResource("new_resource")

	buf := bytes.NewBuffer(nil)
	require.NoError(t, lazy.WriteProto(buf))

	mutated := pb.Span{}
	mutated.Unmarshal(buf.Bytes())

	fmt.Println(string(buf.Bytes()))
	require.Equal(t, "new_type", mutated.Type)
	require.Equal(t, "new_service", mutated.Service)
	require.Equal(t, "new_name", mutated.Name)
	require.Equal(t, "new_resource", mutated.Resource)
}

func newTestSpan() pb.Span {
	return pb.Span{
		TraceID:  42,
		SpanID:   52,
		ParentID: 42,
		Type:     "web",
		Service:  "fennel_IS amazing!",
		Name:     "something &&<@# that should be a metric!",
		Resource: "NOT touched because it is going to be hashed",
		Start:    9223372036854775807,
		Duration: 9223372036854775807,
		Meta:     map[string]string{"http.host": "192.168.0.1"},
		Metrics:  map[string]float64{"http.monitor": 41.99},
	}
}
