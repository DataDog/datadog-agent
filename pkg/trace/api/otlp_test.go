package api

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/pb/otlppb"
	"github.com/stretchr/testify/assert"
)

var testID128 = []byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3}

func TestOTLPHelpers(t *testing.T) {
	t.Run("AnyValueString", func(t *testing.T) {
		for in, out := range map[*otlppb.AnyValue]string{
			{Value: &otlppb.AnyValue_StringValue{StringValue: "string"}}: "string",
			{Value: &otlppb.AnyValue_BoolValue{BoolValue: true}}:         "true",
			{Value: &otlppb.AnyValue_BoolValue{BoolValue: false}}:        "false",
			{Value: &otlppb.AnyValue_IntValue{IntValue: 12}}:             "12",
			{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.12345}}:  "2.12",
			{Value: &otlppb.AnyValue_ArrayValue{
				ArrayValue: &otlppb.ArrayValue{
					Values: []*otlppb.AnyValue{
						{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.12345}},
						{Value: &otlppb.AnyValue_StringValue{StringValue: "string"}},
						{Value: &otlppb.AnyValue_BoolValue{BoolValue: true}},
					},
				},
			}}: "2.12,string,true",
			{Value: &otlppb.AnyValue_KvlistValue{
				KvlistValue: &otlppb.KeyValueList{
					Values: []*otlppb.KeyValue{
						{Key: "key1", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_BoolValue{BoolValue: true}}},
						{Key: "key2", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "string"}}},
					},
				},
			}}: "key1:true,key2:string",
		} {
			t.Run("", func(t *testing.T) {
				assert.Equal(t, out, anyValueString(in))
			})
		}
	})

	t.Run("byteArrayToUint64", func(t *testing.T) {
		assert.Equal(t, uint64(0xa5e7bdf20a52df72), byteArrayToUint64(testID128))
		assert.Equal(t, uint64(0), byteArrayToUint64(nil))
		assert.Equal(t, uint64(0), byteArrayToUint64([]byte{0}))
		assert.Equal(t, uint64(0), byteArrayToUint64([]byte{0, 1, 2, 3, 4, 5, 6}))
	})

	t.Run("spanKindNames", func(t *testing.T) {
		for in, out := range map[otlppb.Span_SpanKind]string{
			otlppb.Span_SPAN_KIND_UNSPECIFIED: "unspecified",
			otlppb.Span_SPAN_KIND_INTERNAL:    "internal",
			otlppb.Span_SPAN_KIND_SERVER:      "server",
			otlppb.Span_SPAN_KIND_CLIENT:      "client",
			otlppb.Span_SPAN_KIND_PRODUCER:    "producer",
			otlppb.Span_SPAN_KIND_CONSUMER:    "consumer",
			99:                                "unknown",
		} {
			assert.Equal(t, out, spanKindName(in))
		}
	})

	t.Run("convertSpan", func(t *testing.T) {
		now := uint64(time.Now().UnixNano())
		for _, tt := range []struct {
			rattr map[string]string
			lib   *otlppb.InstrumentationLibrary
			in    *otlppb.Span
			out   *pb.Span
		}{
			{
				rattr: map[string]string{
					"service.name":    "pylons",
					"service.version": "v1.2.3",
					"env":             "staging",
				},
				lib: &otlppb.InstrumentationLibrary{
					Name:    "ddtracer",
					Version: "v2",
				},
				in: &otlppb.Span{
					TraceId:           testID128,
					SpanId:            testID128,
					TraceState:        "state",
					ParentSpanId:      []byte{0},
					Name:              "/path",
					Kind:              otlppb.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: now,
					EndTimeUnixNano:   now + 200000000,
					Attributes: []*otlppb.KeyValue{
						{Key: "abc", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "qwe"}}},
					},
					DroppedAttributesCount: 0,
					Events:                 nil,
					DroppedEventsCount:     0,
					Links:                  nil,
					DroppedLinksCount:      0,
					Status: &otlppb.Status{
						Message: "OK",
						Code:    otlppb.Status_STATUS_CODE_OK,
					},
				},
				out: &pb.Span{
					Service:  "pylons",
					Name:     "ddtracer.server",
					Resource: "/path",
					TraceID:  11954732583131209586,
					SpanID:   11954732583131209586,
					ParentID: 0,
					Start:    int64(now),
					Duration: 200000000,
					Error:    0,
					Meta: map[string]string{
						"abc":                             "qwe",
						"env":                             "staging",
						"instrumentation_library.name":    "ddtracer",
						"instrumentation_library.version": "v2",
						"otlp_ids.parent":                 "00",
						"otlp_ids.span":                   "72df520af2bde7a5240031ead750e5f3",
						"otlp_ids.trace":                  "72df520af2bde7a5240031ead750e5f3",
						"service.name":                    "pylons",
						"service.version":                 "v1.2.3",
						"trace_state":                     "state",
						"version":                         "v1.2.3",
					},
					Metrics: map[string]float64{"_sampling_priority_v1": 1},
					Type:    "web",
				},
			},
		} {
			assert.Equal(t, tt.out, convertSpan(tt.rattr, tt.lib, tt.in))
		}
	})
}
