// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/pb/otlppb"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var otlpTestID128 = []byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3}

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

func TestOTLPIngest(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("passthrough", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithInsecure())
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := otlppb.NewTraceServiceClient(conn)
		pack := otlppb.ExportTraceServiceRequest{
			ResourceSpans: []*otlppb.ResourceSpans{
				{
					Resource: &otlppb.Resource{
						Attributes: []*otlppb.KeyValue{
							{Key: "service.name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "pylons"}}},
						},
					},
					InstrumentationLibrarySpans: []*otlppb.InstrumentationLibrarySpans{
						{
							InstrumentationLibrary: &otlppb.InstrumentationLibrary{Name: "test", Version: "0.1t"},
							Spans:                  []*otlppb.Span{makeOTLPTestSpan(uint64(time.Now().UnixNano()))},
						},
					},
				},
			},
		}
		_, err = client.Export(context.Background(), &pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		waitForTrace(t, &r, func(p pb.AgentPayload) {
			assert := assert.New(t)
			assert.Equal(p.Env, "my-env")
			assert.Len(p.TracerPayloads, 1)
			assert.Len(p.TracerPayloads[0].Chunks, 1)
			assert.Len(p.TracerPayloads[0].Chunks[0].Spans, 1)
			assert.Equal(p.TracerPayloads[0].Chunks[0].Spans[0].Meta["name"], "john")
		})
	})

	// regression test for DataDog/datadog-agent#11297
	t.Run("duplicate-spanID", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithInsecure())
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := otlppb.NewTraceServiceClient(conn)
		pack := otlppb.ExportTraceServiceRequest{
			ResourceSpans: []*otlppb.ResourceSpans{
				{
					Resource: &otlppb.Resource{
						Attributes: []*otlppb.KeyValue{
							{Key: "service.name", Value: &otlppb.AnyValue{Value: &otlppb.AnyValue_StringValue{StringValue: "pylons"}}},
						},
					},
					InstrumentationLibrarySpans: []*otlppb.InstrumentationLibrarySpans{
						{
							InstrumentationLibrary: &otlppb.InstrumentationLibrary{Name: "test", Version: "0.1t"},
							Spans: []*otlppb.Span{
								makeOTLPTestSpan(uint64(time.Now().UnixNano())),
								makeOTLPTestSpan(uint64(time.Now().UnixNano())),
							},
						},
					},
				},
			},
		}
		_, err = client.Export(context.Background(), &pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		timeout := time.After(1 * time.Second)
	loop:
		for {
			select {
			case <-timeout:
				t.Fatal("Timed out waiting for duplicate SpanID warning.")
			default:
				time.Sleep(10 * time.Millisecond)
				if strings.Contains(r.AgentLog(), `Found malformed trace with duplicate span ID (reason:duplicate_span_id): service:"pylons"`) {
					break loop
				}
			}
		}
	})
}
