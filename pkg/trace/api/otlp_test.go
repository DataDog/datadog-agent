package api

import (
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/pb/otlppb"
	"github.com/stretchr/testify/assert"
)

func makeOTLPTestSpan(start uint64) *otlppb.Span {
	return &otlppb.Span{
		TraceId:           otlpTestID128,
		SpanId:            otlpTestID128,
		TraceState:        "state",
		ParentSpanId:      []byte{0},
		Name:              "/path",
		Kind:              otlppb.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: start,
		EndTimeUnixNano:   start + 200000000,
		Attributes: []*otlppb.KeyValue{
			{Key: "name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "john"}}},
			{Key: "name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 1.2}}},
			{Key: "count", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_IntValue{IntValue: 2}}},
		},
		DroppedAttributesCount: 0,
		Events: []*otlppb.Span_Event{
			{
				TimeUnixNano: 123,
				Name:         "boom",
				Attributes: []*otlppb.KeyValue{
					{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
					{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
				},
				DroppedAttributesCount: 2,
			},
			{
				TimeUnixNano: 456,
				Name:         "exception",
				Attributes: []*otlppb.KeyValue{
					{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
					{Key: "exception.type", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mem"}}},
					{Key: "exception.stacktrace", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "1/2/3"}}},
				},
				DroppedAttributesCount: 2,
			},
		},
		DroppedEventsCount: 0,
		Links:              nil,
		DroppedLinksCount:  0,
		Status: &otlppb.Status{
			Message: "Error",
			Code:    otlppb.Status_STATUS_CODE_ERROR,
		},
	}
}

var (
	// otlpTestID128 is an Opentelemetry compatible 128-bit ID represented as a 16-element byte array.
	otlpTestID128 = []byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3}
	// otlpTestTraceServiceReq holds a basic trace request used for testing.
	otlpTestTraceServiceReq = &otlppb.ExportTraceServiceRequest{
		ResourceSpans: []*otlppb.ResourceSpans{
			{
				Resource: &otlppb.Resource{
					Attributes: []*otlppb.KeyValue{
						{Key: "service.name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mongodb"}}},
						{Key: "binary", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "rundb"}}},
					},
					DroppedAttributesCount: 2,
				},
				InstrumentationLibrarySpans: []*otlppb.InstrumentationLibrarySpans{
					{
						InstrumentationLibrary: &otlppb.InstrumentationLibrary{
							Name:    "libname",
							Version: "v1.2.3",
						},
						Spans: []*otlppb.Span{makeOTLPTestSpan(uint64(time.Now().UnixNano()))},
					},
				},
			},
			{
				Resource: &otlppb.Resource{
					Attributes: []*otlppb.KeyValue{
						{Key: "service.name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "pylons"}}},
						{Key: "binary", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "runweb"}}},
					},
					DroppedAttributesCount: 1,
				},
				InstrumentationLibrarySpans: []*otlppb.InstrumentationLibrarySpans{
					{
						InstrumentationLibrary: &otlppb.InstrumentationLibrary{
							Name:    "othername",
							Version: "v1.2.0",
						},
						Spans: []*otlppb.Span{makeOTLPTestSpan(uint64(time.Now().UnixNano()))},
					},
				},
			},
		},
	}
)

func TestOTLPReceiver(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		o := NewOTLPReceiver(nil, nil)
		assert.NotNil(t, o.cfg)
	})

	t.Run("Start/nil", func(t *testing.T) {
		o := NewOTLPReceiver(nil, nil)
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.httpsrv)
		assert.Nil(t, o.grpcsrv)
	})

	t.Run("Start/http", func(t *testing.T) {
		o := NewOTLPReceiver(nil, &config.OTLP{
			BindHost: "localhost",
			HTTPPort: 50052,
		})
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.grpcsrv)
		assert.NotNil(t, o.httpsrv)
		assert.Equal(t, "localhost:50052", o.httpsrv.Addr)
	})

	t.Run("Start/grpc", func(t *testing.T) {
		o := NewOTLPReceiver(nil, &config.OTLP{
			BindHost: "localhost",
			GRPCPort: 50051,
		})
		o.Start()
		defer o.Stop()
		assert := assert.New(t)
		assert.Nil(o.httpsrv)
		assert.NotNil(o.grpcsrv)
		svc, ok := o.grpcsrv.GetServiceInfo()["opentelemetry.proto.collector.trace.v1.TraceService"]
		assert.True(ok)
		assert.Equal("trace_service.proto", svc.Metadata)
		assert.Equal("Export", svc.Methods[0].Name)
	})

	t.Run("Start/http+grpc", func(t *testing.T) {
		o := NewOTLPReceiver(nil, &config.OTLP{
			BindHost: "localhost",
			HTTPPort: 50052,
			GRPCPort: 50051,
		})
		o.Start()
		defer o.Stop()
		assert.NotNil(t, o.grpcsrv)
		assert.NotNil(t, o.httpsrv)
	})

	t.Run("processRequest", func(t *testing.T) {
		out := make(chan *Payload, 5)
		o := NewOTLPReceiver(out, nil)
		o.processRequest(otlpProtocolGRPC, http.Header(map[string][]string{
			headerLang:        {"go"},
			headerContainerID: {"containerdID"},
		}), otlpTestTraceServiceReq)
		ps := make([]*Payload, 2)
		timeout := time.After(time.Second / 2)
		for i := 0; i < 2; i++ {
			select {
			case p := <-out:
				assert.Equal(t, "go", p.Source.Lang)
				assert.Equal(t, "opentelemetry_grpc_v1", p.Source.EndpointVersion)
				assert.Len(t, p.Traces, 1)
				ps[i] = p
			case <-timeout:
				t.Fatal("timed out")
			}
		}
	})
}

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
		assert.Equal(t, uint64(0x240031ead750e5f3), byteArrayToUint64(otlpTestID128))
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

	t.Run("status2Error", func(t *testing.T) {
		for _, tt := range []struct {
			status *otlppb.Status
			events []*otlppb.Span_Event
			out    pb.Span
		}{
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_ERROR},
				events: []*otlppb.Span_Event{
					{
						Name: "exception",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
							{Key: "exception.type", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mem"}}},
							{Key: "exception.stacktrace", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "1/2/3"}}},
						},
					},
				},
				out: pb.Span{
					Error: 1,
					Meta: map[string]string{
						"error.msg":   "Out of memory",
						"error.type":  "mem",
						"error.stack": "1/2/3",
					},
				},
			},
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_ERROR},
				events: []*otlppb.Span_Event{
					{
						Name: "exception",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
						},
					},
				},
				out: pb.Span{
					Error: 1,
					Meta:  map[string]string{"error.msg": "Out of memory"},
				},
			},
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_ERROR},
				events: []*otlppb.Span_Event{
					{
						Name: "EXCEPTION",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
						},
					},
				},
				out: pb.Span{
					Error: 1,
					Meta:  map[string]string{"error.msg": "Out of memory"},
				},
			},
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_ERROR},
				events: []*otlppb.Span_Event{
					{
						Name: "OTher",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
						},
					},
				},
				out: pb.Span{Error: 1},
			},
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_ERROR},
				out:    pb.Span{Error: 1},
			},
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_OK},
				out:    pb.Span{Error: 0},
			},
			{
				status: &otlppb.Status{Code: otlppb.Status_STATUS_CODE_OK},
				events: []*otlppb.Span_Event{
					{
						Name: "exception",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
							{Key: "exception.type", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mem"}}},
							{Key: "exception.stacktrace", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "1/2/3"}}},
						},
					},
				},
				out: pb.Span{Error: 0},
			},
		} {
			assert := assert.New(t)
			span := pb.Span{Meta: make(map[string]string)}
			status2Error(tt.status, tt.events, &span)
			assert.Equal(tt.out.Error, span.Error)
			for _, prop := range []string{"error.msg", "error.type", "error.stack"} {
				if v, ok := tt.out.Meta[prop]; ok {
					assert.Equal(v, span.Meta[prop])
				} else {
					_, ok := span.Meta[prop]
					assert.False(ok, prop)
				}
			}
		}
	})

	t.Run("resourceFromTags", func(t *testing.T) {
		for _, tt := range []struct {
			meta map[string]string
			out  string
		}{
			{
				meta: nil,
				out:  "",
			},
			{
				meta: map[string]string{"http.method": "GET"},
				out:  "GET",
			},
			{
				meta: map[string]string{"http.method": "POST", "http.route": "/settings"},
				out:  "POST /settings",
			},
			{
				meta: map[string]string{"http.method": "POST", "grpc.path": "/settings"},
				out:  "POST /settings",
			},
			{
				meta: map[string]string{"messaging.operation": "DO"},
				out:  "DO",
			},
			{
				meta: map[string]string{"messaging.operation": "DO", "messaging.destination": "OP"},
				out:  "DO OP",
			},
		} {
			assert.Equal(t, tt.out, resourceFromTags(tt.meta))
		}
	})

	t.Run("spanKind2Type", func(t *testing.T) {
		for _, tt := range []struct {
			kind otlppb.Span_SpanKind
			meta map[string]string
			out  string
		}{
			{
				kind: otlppb.Span_SPAN_KIND_SERVER,
				out:  "web",
			},
			{
				kind: otlppb.Span_SPAN_KIND_CLIENT,
				out:  "http",
			},
			{
				kind: otlppb.Span_SPAN_KIND_CLIENT,
				meta: map[string]string{"db.system": "redis"},
				out:  "cache",
			},
			{
				kind: otlppb.Span_SPAN_KIND_CLIENT,
				meta: map[string]string{"db.system": "memcached"},
				out:  "cache",
			},
			{
				kind: otlppb.Span_SPAN_KIND_CLIENT,
				meta: map[string]string{"db.system": "other"},
				out:  "db",
			},
			{
				kind: otlppb.Span_SPAN_KIND_PRODUCER,
				out:  "custom",
			},
			{
				kind: otlppb.Span_SPAN_KIND_CONSUMER,
				out:  "custom",
			},
			{
				kind: otlppb.Span_SPAN_KIND_INTERNAL,
				out:  "custom",
			},
			{
				kind: otlppb.Span_SPAN_KIND_UNSPECIFIED,
				out:  "custom",
			},
		} {
			assert.Equal(t, tt.out, spanKind2Type(tt.kind, &pb.Span{Meta: tt.meta}))
		}
	})

	t.Run("tagsFromHeaders", func(t *testing.T) {
		out := tagsFromHeaders(http.Header(map[string][]string{
			headerLang:                  {"go"},
			headerLangVersion:           {"1.14"},
			headerLangInterpreter:       {"x"},
			headerLangInterpreterVendor: {"y"},
		}), otlpProtocolGRPC)
		assert.Equal(t, []string{"endpoint_version:opentelemetry_grpc_v1", "lang:go", "lang_version:1.14", "interpreter:x", "lang_vendor:y"}, out)
	})
}

func TestOTLPConvertSpan(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	for i, tt := range []struct {
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
			in: makeOTLPTestSpan(now),
			out: &pb.Span{
				Service:  "pylons",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                            "john",
					"env":                             "staging",
					"instrumentation_library.name":    "ddtracer",
					"instrumentation_library.version": "v2",
					"service.name":                    "pylons",
					"service.version":                 "v1.2.3",
					"trace_state":                     "state",
					"version":                         "v1.2.3",
					"events":                          "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":\"2.40\"},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"error.msg":                       "Out of memory",
					"error.type":                      "mem",
					"error.stack":                     "1/2/3",
				},
				Metrics: map[string]float64{
					"_sampling_priority_v1": 1,
					"name":                  1.2,
					"count":                 2,
				},
				Type: "web",
			},
		}, {
			rattr: map[string]string{
				"service.version": "v1.2.3",
			},
			lib: &otlppb.InstrumentationLibrary{
				Name:    "ddtracer",
				Version: "v2",
			},
			in: &otlppb.Span{
				TraceId:           otlpTestID128,
				SpanId:            otlpTestID128,
				TraceState:        "state",
				ParentSpanId:      []byte{0},
				Name:              "/path",
				Kind:              otlppb.Span_SPAN_KIND_SERVER,
				StartTimeUnixNano: now,
				EndTimeUnixNano:   now + 200000000,
				Attributes: []*otlppb.KeyValue{
					{Key: "name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "john"}}},
					{Key: "peer.service", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "userbase"}}},
					{Key: "deployment.environment", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "prod"}}},
					{Key: "http.method", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "GET"}}},
					{Key: "http.route", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "/path"}}},
					{Key: "name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 1.2}}},
					{Key: "count", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_IntValue{IntValue: 2}}},
				},
				DroppedAttributesCount: 0,
				Events: []*otlppb.Span_Event{
					{
						TimeUnixNano: 123,
						Name:         "boom",
						Attributes: []*otlppb.KeyValue{
							{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
							{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
						},
						DroppedAttributesCount: 2,
					},
					{
						TimeUnixNano: 456,
						Name:         "exception",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
							{Key: "exception.type", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mem"}}},
							{Key: "exception.stacktrace", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "1/2/3"}}},
						},
						DroppedAttributesCount: 2,
					},
				},
				DroppedEventsCount: 0,
				Links:              nil,
				DroppedLinksCount:  0,
				Status: &otlppb.Status{
					Message: "Error",
					Code:    otlppb.Status_STATUS_CODE_ERROR,
				},
			},
			out: &pb.Span{
				Service:  "userbase",
				Name:     "ddtracer.server",
				Resource: "GET /path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                            "john",
					"env":                             "prod",
					"deployment.environment":          "prod",
					"instrumentation_library.name":    "ddtracer",
					"instrumentation_library.version": "v2",
					"service.version":                 "v1.2.3",
					"trace_state":                     "state",
					"version":                         "v1.2.3",
					"events":                          "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":\"2.40\"},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"error.msg":                       "Out of memory",
					"error.type":                      "mem",
					"error.stack":                     "1/2/3",
					"http.method":                     "GET",
					"http.route":                      "/path",
					"peer.service":                    "userbase",
				},
				Metrics: map[string]float64{
					"_sampling_priority_v1": 1,
					"name":                  1.2,
					"count":                 2,
				},
				Type: "web",
			},
		}, {
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
				TraceId:           otlpTestID128,
				SpanId:            otlpTestID128,
				TraceState:        "state",
				ParentSpanId:      []byte{0},
				Name:              "/path",
				Kind:              otlppb.Span_SPAN_KIND_SERVER,
				StartTimeUnixNano: now,
				EndTimeUnixNano:   now + 200000000,
				Attributes: []*otlppb.KeyValue{
					{Key: "name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "john"}}},
					{Key: "http.method", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "GET"}}},
					{Key: "http.route", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "/path"}}},
					{Key: "name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 1.2}}},
					{Key: "count", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_IntValue{IntValue: 2}}},
				},
				DroppedAttributesCount: 0,
				Events: []*otlppb.Span_Event{
					{
						TimeUnixNano: 123,
						Name:         "boom",
						Attributes: []*otlppb.KeyValue{
							{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
							{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
						},
						DroppedAttributesCount: 2,
					},
					{
						TimeUnixNano: 456,
						Name:         "exception",
						Attributes: []*otlppb.KeyValue{
							{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "Out of memory"}}},
							{Key: "exception.type", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mem"}}},
							{Key: "exception.stacktrace", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "1/2/3"}}},
						},
						DroppedAttributesCount: 2,
					},
				},
				DroppedEventsCount: 0,
				Links:              nil,
				DroppedLinksCount:  0,
				Status: &otlppb.Status{
					Message: "Error",
					Code:    otlppb.Status_STATUS_CODE_ERROR,
				},
			},
			out: &pb.Span{
				Service:  "pylons",
				Name:     "ddtracer.server",
				Resource: "GET /path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                            "john",
					"env":                             "staging",
					"instrumentation_library.name":    "ddtracer",
					"instrumentation_library.version": "v2",
					"service.name":                    "pylons",
					"service.version":                 "v1.2.3",
					"trace_state":                     "state",
					"version":                         "v1.2.3",
					"events":                          "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":\"2.40\"},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"error.msg":                       "Out of memory",
					"error.type":                      "mem",
					"error.stack":                     "1/2/3",
					"http.method":                     "GET",
					"http.route":                      "/path",
				},
				Metrics: map[string]float64{
					"_sampling_priority_v1": 1,
					"name":                  1.2,
					"count":                 2,
				},
				Type: "web",
			},
		},
	} {
		assert.Equal(t, tt.out, convertSpan(tt.rattr, tt.lib, tt.in), i)
	}
}

func TestMarshalEvents(t *testing.T) {
	for _, tt := range []struct {
		in  []*otlppb.Span_Event
		out string
	}{
		{
			in: []*otlppb.Span_Event{
				{
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
					},
					DroppedAttributesCount: 3,
				},
			},
			out: `[{
					"attributes": {"message":"OOM"},
					"dropped_attributes_count":3
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					Name: "boom",
				},
			},
			out: `[{"name":"boom"}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					Name: "boom",
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
					},
					DroppedAttributesCount: 3,
				},
			},
			out: `[{
					"name":"boom",
					"attributes": {"message":"OOM"},
					"dropped_attributes_count":3
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					TimeUnixNano: 123,
					Name:         "boom",
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
					},
					DroppedAttributesCount: 2,
				},
			},
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": { "message":"OOM" },
					"dropped_attributes_count":2
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					DroppedAttributesCount: 2,
				},
			},
			out: `[{"dropped_attributes_count":2}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					TimeUnixNano: 123,
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
						{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
					},
					DroppedAttributesCount: 2,
				},
			},
			out: `[{
					"time_unix_nano":123,
					"attributes": {
						"message":"OOM",
						"accuracy":"2.40"
					},
					"dropped_attributes_count":2
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					TimeUnixNano: 123,
					Name:         "boom",
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
						{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
					},
				},
			},
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"message":"OOM",
						"accuracy":"2.40"
					}
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					TimeUnixNano:           123,
					Name:                   "boom",
					DroppedAttributesCount: 2,
				},
			},
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"dropped_attributes_count":2
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					TimeUnixNano: 123,
					Name:         "boom",
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
						{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
					},
					DroppedAttributesCount: 2,
				},
			},
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"message":"OOM",
						"accuracy":"2.40"
					},
					"dropped_attributes_count":2
				}]`,
		}, {
			in: []*otlppb.Span_Event{
				{
					TimeUnixNano: 123,
					Name:         "boom",
					Attributes: []*otlppb.KeyValue{
						{Key: "message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
						{Key: "accuracy", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_DoubleValue{DoubleValue: 2.4}}},
					},
					DroppedAttributesCount: 2,
				},
				{
					TimeUnixNano: 456,
					Name:         "exception",
					Attributes: []*otlppb.KeyValue{
						{Key: "exception.message", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "OOM"}}},
						{Key: "exception.type", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "mem"}}},
						{Key: "exception.stacktrace", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "1/2/3"}}},
					},
					DroppedAttributesCount: 2,
				},
			},
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"message":"OOM",
						"accuracy":"2.40"
					},
					"dropped_attributes_count":2
				}, {
					"time_unix_nano":456,
					"name":"exception",
					"attributes": {
						"exception.message":"OOM",
						"exception.type":"mem",
						"exception.stacktrace":"1/2/3"
					},
					"dropped_attributes_count":2
				}]`,
		},
	} {
		assert.Equal(t, trimSpaces(tt.out), marshalEvents(tt.in))
	}
}

func trimSpaces(str string) string {
	var out strings.Builder
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			out.WriteRune(ch)
		}
	}
	return out.String()
}

func BenchmarkProcessRequest(b *testing.B) {
	metadata := http.Header(map[string][]string{
		headerLang:        {"go"},
		headerContainerID: {"containerdID"},
	})
	out := make(chan *Payload, 100)
	end := make(chan struct{})
	go func() {
		defer close(end)
		for {
			select {
			case <-out:
				// drain
			case <-end:
				return
			}
		}
	}()

	r := NewOTLPReceiver(out, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.processRequest(otlpProtocolHTTP, metadata, otlpTestTraceServiceReq)
	}
	b.StopTimer()
	end <- struct{}{}
	<-end
}
