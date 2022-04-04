// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	semconv "go.opentelemetry.io/collector/model/semconv/v1.6.1"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var otlpTestSpanConfig = &testutil.OTLPSpan{
	TraceState: "state",
	Name:       "/path",
	Kind:       ptrace.SpanKindServer,
	Attributes: map[string]interface{}{
		"name":   "john",
		"approx": 1.2,
		"count":  2,
	},
	Events: []testutil.OTLPSpanEvent{
		{
			Timestamp: 123,
			Name:      "boom",
			Attributes: map[string]interface{}{
				"key":      "Out of memory",
				"accuracy": 2.4,
			},
			Dropped: 2,
		},
		{
			Timestamp: 456,
			Name:      "exception",
			Attributes: map[string]interface{}{
				"exception.message":    "Out of memory",
				"exception.type":       "mem",
				"exception.stacktrace": "1/2/3",
			},
			Dropped: 2,
		},
	},
	StatusMsg:  "Error",
	StatusCode: ptrace.StatusCodeError,
}

var otlpTestSpan = testutil.NewOTLPSpan(otlpTestSpanConfig)

var otlpTestTracesRequest = testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
	{
		LibName:    "libname",
		LibVersion: "v1.2.3",
		Attributes: map[string]interface{}{
			"service.name": "mongodb",
			"binary":       "rundb",
		},
		Spans: []*testutil.OTLPSpan{otlpTestSpanConfig},
	},
	{
		LibName:    "othername",
		LibVersion: "v1.2.0",
		Attributes: map[string]interface{}{
			"service.name": "pylons",
			"binary":       "runweb",
		},
		Spans: []*testutil.OTLPSpan{otlpTestSpanConfig},
	},
})

func TestOTLPNameRemapping(t *testing.T) {
	cfg := config.New()
	cfg.OTLPReceiver.SpanNameRemappings = map[string]string{"libname.unspecified": "new"}
	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg)
	rcv.ReceiveResourceSpans(testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "asd"},
			},
		},
	}).Traces().ResourceSpans().At(0), http.Header{}, "")
	timeout := time.After(500 * time.Millisecond)
	select {
	case <-timeout:
		t.Fatal("timed out")
	case p := <-out:
		assert.Equal(t, "new", p.TracerPayload.Chunks[0].Spans[0].Name)
	}
}

func TestOTLPReceiveResourceSpans(t *testing.T) {
	cfg := config.New()
	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg)
	require := require.New(t)
	for _, tt := range []struct {
		in []testutil.OTLPResourceSpan
		fn func(*pb.TracerPayload)
	}{
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{string(semconv.AttributeDeploymentEnvironment): "depenv"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("depenv", out.Env)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.AttributeDeploymentEnvironment): "spanenv"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("spanenv", out.Env)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{"_dd.hostname": "dd.host"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("dd.host", out.Hostname)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{string(semconv.AttributeContainerID): "1234cid"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("1234cid", out.ContainerID)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{string(semconv.AttributeK8SPodUID): "1234cid"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("1234cid", out.ContainerID)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.AttributeK8SPodUID): "123cid"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("123cid", out.ContainerID)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.AttributeContainerID): "23cid"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("23cid", out.ContainerID)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					Spans: []*testutil.OTLPSpan{
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "first",
						},
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:  [8]byte{10, 10, 11, 12, 13, 14, 15, 16},
							Name:    "second",
						},
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:  [8]byte{9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "third",
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Len(out.Chunks, 2)
				require.Equal(uint64(0x90a0b0c0d0e0f10), out.Chunks[0].Spans[0].TraceID)
				require.Len(out.Chunks[1].Spans, 2)
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			rcv.ReceiveResourceSpans(testutil.NewOTLPTracesRequest(tt.in).Traces().ResourceSpans().At(0), http.Header{}, "agent_tests")
			timeout := time.After(500 * time.Millisecond)
			select {
			case <-timeout:
				t.Fatal("timed out")
			case p := <-out:
				tt.fn(p.TracerPayload)
			}
		})
	}
}

func TestOTLPHostname(t *testing.T) {
	for _, tt := range []struct {
		config, resource, span string
		out                    string
	}{
		{
			config:   "config-hostname",
			resource: "resource-hostname",
			span:     "span-hostname",
			out:      "resource-hostname",
		},
		{
			config: "config-hostname",
			out:    "config-hostname",
		},
		{
			config: "config-hostname",
			span:   "span-hostname",
			out:    "span-hostname",
		},
	} {
		cfg := config.New()
		cfg.Hostname = tt.config
		out := make(chan *Payload, 1)
		rcv := NewOTLPReceiver(out, cfg)
		rcv.ReceiveResourceSpans(testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "a",
				LibVersion: "1.2",
				Attributes: map[string]interface{}{"datadog.host.name": tt.resource},
				Spans: []*testutil.OTLPSpan{
					{Attributes: map[string]interface{}{"_dd.hostname": tt.span}},
				},
			},
		}).Traces().ResourceSpans().At(0), http.Header{}, "")
		timeout := time.After(500 * time.Millisecond)
		select {
		case <-timeout:
			t.Fatal("timed out")
		case p := <-out:
			assert.Equal(t, tt.out, p.TracerPayload.Hostname)
		}
	}
}

func TestOTLPReceiver(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		cfg := config.New()
		assert.NotNil(t, NewOTLPReceiver(nil, cfg).conf)
	})

	t.Run("Start/nil", func(t *testing.T) {
		o := NewOTLPReceiver(nil, config.New())
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.httpsrv)
		assert.Nil(t, o.grpcsrv)
	})

	t.Run("Start/http", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		cfg := config.New()
		cfg.OTLPReceiver = &config.OTLP{
			BindHost: "localhost",
			HTTPPort: port,
		}
		o := NewOTLPReceiver(nil, cfg)
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.grpcsrv)
		assert.NotNil(t, o.httpsrv)
		assert.Equal(t, fmt.Sprintf("localhost:%d", port), o.httpsrv.Addr)
	})

	t.Run("Start/grpc", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		cfg := config.New()
		cfg.OTLPReceiver = &config.OTLP{
			BindHost: "localhost",
			GRPCPort: port,
		}
		o := NewOTLPReceiver(nil, cfg)
		o.Start()
		defer o.Stop()
		assert := assert.New(t)
		assert.Nil(o.httpsrv)
		assert.NotNil(o.grpcsrv)
		svc, ok := o.grpcsrv.GetServiceInfo()["opentelemetry.proto.collector.trace.v1.TraceService"]
		assert.True(ok)
		assert.Equal("opentelemetry/proto/collector/trace/v1/trace_service.proto", svc.Metadata)
		assert.Equal("Export", svc.Methods[0].Name)
	})

	t.Run("Start/http+grpc", func(t *testing.T) {
		port1, port2 := testutil.FreeTCPPort(t), testutil.FreeTCPPort(t)
		cfg := config.New()
		cfg.OTLPReceiver = &config.OTLP{
			BindHost: "localhost",
			HTTPPort: port1,
			GRPCPort: port2,
		}
		o := NewOTLPReceiver(nil, cfg)
		o.Start()
		defer o.Stop()
		assert.NotNil(t, o.grpcsrv)
		assert.NotNil(t, o.httpsrv)
	})

	t.Run("processRequest", func(t *testing.T) {
		out := make(chan *Payload, 5)
		o := NewOTLPReceiver(out, config.New())
		o.processRequest(otlpProtocolGRPC, http.Header(map[string][]string{
			headerLang:        {"go"},
			headerContainerID: {"containerdID"},
		}), otlpTestTracesRequest)
		ps := make([]*Payload, 2)
		timeout := time.After(time.Second / 2)
		for i := 0; i < 2; i++ {
			select {
			case p := <-out:
				assert.Equal(t, "go", p.Source.Lang)
				assert.Equal(t, "opentelemetry_grpc_v1", p.Source.EndpointVersion)
				assert.Len(t, p.TracerPayload.Chunks, 1)
				ps[i] = p
			case <-timeout:
				t.Fatal("timed out")
			}
		}
	})
}

var (
	otlpTestSpanID  = pcommon.NewSpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
	otlpTestTraceID = pcommon.NewTraceID([16]byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
)

func TestOTLPHelpers(t *testing.T) {
	t.Run("byteArrayToUint64", func(t *testing.T) {
		assert.Equal(t, uint64(0x240031ead750e5f3), traceIDToUint64(otlpTestTraceID.Bytes()))
		assert.Equal(t, uint64(0x240031ead750e5f3), spanIDToUint64(otlpTestSpanID.Bytes()))
	})

	t.Run("spanKindNames", func(t *testing.T) {
		for in, out := range map[ptrace.SpanKind]string{
			ptrace.SpanKindUnspecified: "unspecified",
			ptrace.SpanKindInternal:    "internal",
			ptrace.SpanKindServer:      "server",
			ptrace.SpanKindClient:      "client",
			ptrace.SpanKindProducer:    "producer",
			ptrace.SpanKindConsumer:    "consumer",
			99:                         "unknown",
		} {
			assert.Equal(t, out, spanKindName(in))
		}
	})

	t.Run("status2Error", func(t *testing.T) {
		for _, tt := range []struct {
			status ptrace.StatusCode
			msg    string
			events ptrace.SpanEventSlice
			out    pb.Span
		}{
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("exception", map[string]string{
					"exception.message":    "Out of memory",
					"exception.type":       "mem",
					"exception.stacktrace": "1/2/3",
				}, 0, 0),
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
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("exception", map[string]string{
					"exception.message": "Out of memory",
				}, 0, 0),
				out: pb.Span{
					Error: 1,
					Meta:  map[string]string{"error.msg": "Out of memory"},
				},
			},
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("EXCEPTION", map[string]string{
					"exception.message": "Out of memory",
				}, 0, 0),
				out: pb.Span{
					Error: 1,
					Meta:  map[string]string{"error.msg": "Out of memory"},
				},
			},
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("OTher", map[string]string{
					"exception.message": "Out of memory",
				}, 0, 0),
				out: pb.Span{Error: 1},
			},
			{
				status: ptrace.StatusCodeError,
				events: ptrace.NewSpanEventSlice(),
				out:    pb.Span{Error: 1},
			},
			{
				status: ptrace.StatusCodeError,
				msg:    "Error number #24",
				events: ptrace.NewSpanEventSlice(),
				out:    pb.Span{Error: 1, Meta: map[string]string{"error.msg": "Error number #24"}},
			},
			{
				status: ptrace.StatusCodeOk,
				events: ptrace.NewSpanEventSlice(),
				out:    pb.Span{Error: 0},
			},
			{
				status: ptrace.StatusCodeOk,
				events: makeEventsSlice("exception", map[string]string{
					"exception.message":    "Out of memory",
					"exception.type":       "mem",
					"exception.stacktrace": "1/2/3",
				}, 0, 0),
				out: pb.Span{Error: 0},
			},
		} {
			assert := assert.New(t)
			span := pb.Span{Meta: make(map[string]string)}
			status := ptrace.NewSpanStatus()
			status.SetCode(tt.status)
			status.SetMessage(tt.msg)
			status2Error(status, tt.events, &span)
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
			{
				meta: map[string]string{semconv.AttributeRPCService: "SVC", semconv.AttributeRPCMethod: "M"},
				out:  "M SVC",
			},
			{
				meta: map[string]string{semconv.AttributeRPCMethod: "M"},
				out:  "M",
			},
		} {
			assert.Equal(t, tt.out, resourceFromTags(tt.meta))
		}
	})

	t.Run("spanKind2Type", func(t *testing.T) {
		for _, tt := range []struct {
			kind ptrace.SpanKind
			meta map[string]string
			out  string
		}{
			{
				kind: ptrace.SpanKindServer,
				out:  "web",
			},
			{
				kind: ptrace.SpanKindClient,
				out:  "http",
			},
			{
				kind: ptrace.SpanKindClient,
				meta: map[string]string{"db.system": "redis"},
				out:  "cache",
			},
			{
				kind: ptrace.SpanKindClient,
				meta: map[string]string{"db.system": "memcached"},
				out:  "cache",
			},
			{
				kind: ptrace.SpanKindClient,
				meta: map[string]string{"db.system": "other"},
				out:  "db",
			},
			{
				kind: ptrace.SpanKindProducer,
				out:  "custom",
			},
			{
				kind: ptrace.SpanKindConsumer,
				out:  "custom",
			},
			{
				kind: ptrace.SpanKindInternal,
				out:  "custom",
			},
			{
				kind: ptrace.SpanKindUnspecified,
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
	now := uint64(otlpTestSpan.StartTimestamp())
	cfg := config.New()
	o := NewOTLPReceiver(nil, cfg)
	for i, tt := range []struct {
		rattr   map[string]string
		libname string
		libver  string
		in      ptrace.Span
		out     *pb.Span
	}{
		{
			rattr: map[string]string{
				"service.name":    "pylons",
				"service.version": "v1.2.3",
				"env":             "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in:      otlpTestSpan,
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
					"name":                    "john",
					"otel.trace_id":           "72df520af2bde7a5240031ead750e5f3",
					"env":                     "staging",
					"otel.status_code":        "STATUS_CODE_ERROR",
					"otel.status_description": "Error",
					"otel.library.name":       "ddtracer",
					"otel.library.version":    "v2",
					"service.name":            "pylons",
					"service.version":         "v1.2.3",
					"w3c.tracestate":          "state",
					"version":                 "v1.2.3",
					"events":                  `[{"time_unix_nano":123,"name":"boom","attributes":{"key":"Out of memory","accuracy":"2.4"},"dropped_attributes_count":2},{"time_unix_nano":456,"name":"exception","attributes":{"exception.message":"Out of memory","exception.type":"mem","exception.stacktrace":"1/2/3"},"dropped_attributes_count":2}]`,
					"error.msg":               "Out of memory",
					"error.type":              "mem",
					"error.stack":             "1/2/3",
				},
				Metrics: map[string]float64{
					"approx": 1.2,
					"count":  2,
				},
				Type: "web",
			},
		}, {
			rattr: map[string]string{
				"service.version": "v1.2.3",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID:    otlpTestTraceID.Bytes(),
				SpanID:     otlpTestSpanID.Bytes(),
				TraceState: "state",
				Name:       "/path",
				Kind:       ptrace.SpanKindServer,
				Start:      now,
				End:        now + 200000000,
				Attributes: map[string]interface{}{
					"name":                   "john",
					"peer.service":           "userbase",
					"deployment.environment": "prod",
					"http.method":            "GET",
					"http.route":             "/path",
					"approx":                 1.2,
					"count":                  2,
				},
				Events: []testutil.OTLPSpanEvent{
					{
						Timestamp: 123,
						Name:      "boom",
						Attributes: map[string]interface{}{
							"message":  "Out of memory",
							"accuracy": 2.4,
						},
						Dropped: 2,
					},
					{
						Timestamp: 456,
						Name:      "exception",
						Attributes: map[string]interface{}{
							"exception.message":    "Out of memory",
							"exception.type":       "mem",
							"exception.stacktrace": "1/2/3",
						},
						Dropped: 2,
					},
				},
				StatusMsg:  "Error",
				StatusCode: ptrace.StatusCodeError,
			}),
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
					"name":                    "john",
					"env":                     "prod",
					"deployment.environment":  "prod",
					"otel.trace_id":           "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":        "STATUS_CODE_ERROR",
					"otel.status_description": "Error",
					"otel.library.name":       "ddtracer",
					"otel.library.version":    "v2",
					"service.version":         "v1.2.3",
					"w3c.tracestate":          "state",
					"version":                 "v1.2.3",
					"events":                  "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":\"2.4\"},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"error.msg":               "Out of memory",
					"error.type":              "mem",
					"error.stack":             "1/2/3",
					"http.method":             "GET",
					"http.route":              "/path",
					"peer.service":            "userbase",
				},
				Metrics: map[string]float64{
					"approx": 1.2,
					"count":  2,
				},
				Type: "web",
			},
		}, {
			rattr: map[string]string{
				"service.name":    "pylons",
				"service.version": "v1.2.3",
				"env":             "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID:    otlpTestTraceID.Bytes(),
				SpanID:     otlpTestSpanID.Bytes(),
				TraceState: "state",
				Name:       "/path",
				Kind:       ptrace.SpanKindServer,
				Start:      now,
				End:        now + 200000000,
				Attributes: map[string]interface{}{
					"name":            "john",
					"http.method":     "GET",
					"http.route":      "/path",
					"approx":          1.2,
					"count":           2,
					"analytics.event": "false",
				},
				Events: []testutil.OTLPSpanEvent{
					{
						Timestamp: 123,
						Name:      "boom",
						Attributes: map[string]interface{}{
							"message":  "Out of memory",
							"accuracy": 2.4,
						},
						Dropped: 2,
					},
					{
						Timestamp: 456,
						Name:      "exception",
						Attributes: map[string]interface{}{
							"exception.message":    "Out of memory",
							"exception.type":       "mem",
							"exception.stacktrace": "1/2/3",
						},
						Dropped: 2,
					},
				},
				StatusMsg:  "Error",
				StatusCode: ptrace.StatusCodeError,
			}),
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
					"name":                    "john",
					"env":                     "staging",
					"otel.status_code":        "STATUS_CODE_ERROR",
					"otel.status_description": "Error",
					"otel.library.name":       "ddtracer",
					"otel.library.version":    "v2",
					"service.name":            "pylons",
					"service.version":         "v1.2.3",
					"w3c.tracestate":          "state",
					"version":                 "v1.2.3",
					"otel.trace_id":           "72df520af2bde7a5240031ead750e5f3",
					"events":                  "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":\"2.4\"},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"error.msg":               "Out of memory",
					"error.type":              "mem",
					"error.stack":             "1/2/3",
					"http.method":             "GET",
					"http.route":              "/path",
				},
				Metrics: map[string]float64{
					"approx":                               1.2,
					"count":                                2,
					sampler.KeySamplingRateEventExtraction: 0,
				},
				Type: "web",
			},
		}, {
			rattr: map[string]string{
				"env": "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				Name:  "/path",
				Start: now,
				End:   now + 200000000,
				Attributes: map[string]interface{}{
					"service.name":                    "mongo",
					"operation.name":                  "READ",
					"resource.name":                   "/path",
					"span.type":                       "db",
					"name":                            "john",
					semconv.AttributeContainerID:      "cid",
					semconv.AttributeK8SContainerName: "k8s-container",
					"http.method":                     "GET",
					"http.route":                      "/path",
					"approx":                          1.2,
					"count":                           2,
					"analytics.event":                 true,
				},
			}),
			out: &pb.Span{
				Service:  "mongo",
				Name:     "READ",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                             "staging",
					"_dd.tags.container":              "container_id:cid,kube_container_name:k8s-container",
					semconv.AttributeContainerID:      "cid",
					semconv.AttributeK8SContainerName: "k8s-container",
					"http.method":                     "GET",
					"http.route":                      "/path",
					"otel.status_code":                "STATUS_CODE_UNSET",
					"otel.library.name":               "ddtracer",
					"otel.library.version":            "v2",
					"name":                            "john",
					"otel.trace_id":                   "72df520af2bde7a5240031ead750e5f3",
				},
				Metrics: map[string]float64{
					"approx":                               1.2,
					"count":                                2,
					sampler.KeySamplingRateEventExtraction: 1,
				},
				Type: "db",
			},
		},
	} {
		lib := pcommon.NewInstrumentationScope()
		lib.SetName(tt.libname)
		lib.SetVersion(tt.libver)
		assert := assert.New(t)
		want := tt.out
		got := o.convertSpan(tt.rattr, lib, tt.in)
		if len(want.Meta) != len(got.Meta) {
			t.Fatalf("(%d) Meta count mismatch:\n%#v", i, got.Meta)
		}
		for k, v := range want.Meta {
			switch k {
			case "events":
				// events contain maps with no guaranteed order of
				// traversal; best to unpack to compare
				var gote, wante []testutil.OTLPSpanEvent
				if err := json.Unmarshal([]byte(v), &wante); err != nil {
					t.Fatalf("(%d) Error unmarshalling: %v", i, err)
				}
				if err := json.Unmarshal([]byte(got.Meta[k]), &gote); err != nil {
					t.Fatalf("(%d) Error unmarshalling: %v", i, err)
				}
				assert.Equal(wante, gote)
			case "_dd.container_tags":
				// order not guaranteed, so we need to unpack and sort to compare
				gott := strings.Split(got.Meta[tagContainersTags], ",")
				wantt := strings.Split(want.Meta[tagContainersTags], ",")
				sort.Strings(gott)
				sort.Strings(wantt)
				assert.Equal(wantt, gott)
			default:
				assert.Equal(v, got.Meta[k], fmt.Sprintf("(%d) Meta %v:%v", i, k, v))
			}
		}
		if len(want.Metrics) != len(got.Metrics) {
			t.Fatalf("(%d) Metrics count mismatch:\n\n%v\n\n%v", i, want.Metrics, got.Metrics)
		}
		for k, v := range want.Metrics {
			assert.Equal(v, got.Metrics[k], fmt.Sprintf("(%d) Metric %v:%v", i, k, v))
		}
		want.Meta = nil
		want.Metrics = nil
		got.Meta = nil
		got.Metrics = nil
		assert.Equal(want, got, i)
	}
}

// TestResourceAttributesMap is a regression test ensuring that the resource attributes map
// passed to convertSpan is not modified by it.
func TestResourceAttributesMap(t *testing.T) {
	rattr := map[string]string{"key": "val"}
	lib := pcommon.NewInstrumentationScope()
	span := testutil.NewOTLPSpan(&testutil.OTLPSpan{})
	NewOTLPReceiver(nil, config.New()).convertSpan(rattr, lib, span)
	assert.Len(t, rattr, 1) // ensure "rattr" has no new entries
	assert.Equal(t, "val", rattr["key"])
}

func makeEventsSlice(name string, attrs map[string]string, timestamp int, dropped uint32) ptrace.SpanEventSlice {
	s := ptrace.NewSpanEventSlice()
	e := s.AppendEmpty()
	e.SetName(name)
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e.Attributes().Insert(k, pcommon.NewValueString(attrs[k]))
	}
	e.SetTimestamp(pcommon.Timestamp(timestamp))
	e.SetDroppedAttributesCount(dropped)
	return s
}

func TestMarshalEvents(t *testing.T) {
	for _, tt := range []struct {
		in  ptrace.SpanEventSlice
		out string
	}{
		{
			in: makeEventsSlice("", map[string]string{
				"message": "OOM",
			}, 0, 3),
			out: `[{
					"attributes": {"message":"OOM"},
					"dropped_attributes_count":3
				}]`,
		}, {
			in:  makeEventsSlice("boom", nil, 0, 0),
			out: `[{"name":"boom"}]`,
		}, {
			in: makeEventsSlice("boom", map[string]string{
				"message": "OOM",
			}, 0, 3),
			out: `[{
					"name":"boom",
					"attributes": {"message":"OOM"},
					"dropped_attributes_count":3
				}]`,
		}, {
			in: makeEventsSlice("boom", map[string]string{
				"message": "OOM",
			}, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": { "message":"OOM" },
					"dropped_attributes_count":2
				}]`,
		}, {
			in:  makeEventsSlice("", nil, 0, 2),
			out: `[{"dropped_attributes_count":2}]`,
		}, {
			in: makeEventsSlice("", map[string]string{
				"message":  "OOM",
				"accuracy": "2.40",
			}, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"attributes": {
						"accuracy":"2.40",
						"message":"OOM"
					},
					"dropped_attributes_count":2
				}]`,
		}, {
			in: makeEventsSlice("boom", map[string]string{
				"message":  "OOM",
				"accuracy": "2.40",
			}, 123, 0),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"accuracy":"2.40",
						"message":"OOM"
					}
				}]`,
		}, {
			in: makeEventsSlice("boom", nil, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"dropped_attributes_count":2
				}]`,
		}, {
			in: makeEventsSlice("boom", map[string]string{
				"message":  "OOM",
				"accuracy": "2.4",
			}, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"accuracy":"2.4",
						"message":"OOM"
					},
					"dropped_attributes_count":2
				}]`,
		}, {
			in: (func() ptrace.SpanEventSlice {
				e1 := makeEventsSlice("boom", map[string]string{
					"message":  "OOM",
					"accuracy": "2.4",
				}, 123, 2)
				e2 := makeEventsSlice("exception", map[string]string{
					"exception.message":    "OOM",
					"exception.stacktrace": "1/2/3",
					"exception.type":       "mem",
				}, 456, 2)
				e2.MoveAndAppendTo(e1)
				return e1
			})(),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"accuracy":"2.4",
						"message":"OOM"
					},
					"dropped_attributes_count":2
				}, {
					"time_unix_nano":456,
					"name":"exception",
					"attributes": {
						"exception.message":"OOM",
						"exception.stacktrace":"1/2/3",
						"exception.type":"mem"
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
		r.processRequest(otlpProtocolHTTP, metadata, otlpTestTracesRequest)
	}
	b.StopTimer()
	end <- struct{}{}
	<-end
}
