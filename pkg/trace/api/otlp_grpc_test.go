// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
)

func TestOTLPMetrics(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPMetrics(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPMetrics(true, t)
	})
}

func testOTLPMetrics(enableReceiveResourceSpansV2 bool, t *testing.T) {
	t.Helper()
	assert := assert.New(t)
	cfg := NewTestConfig(t)
	cfg.AgentVersion = "v1.0.0"
	cfg.Hostname = "test-host"
	if !enableReceiveResourceSpansV2 {
		cfg.Features["disable_receive_resource_spans_v2"] = struct{}{}
	}
	stats := &teststatsd.Client{}

	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, stats, &timing.NoopReporter{})
	req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "1"},
				{Name: "2"},
				{Name: "3"},
			},
		},
		{
			LibName:    "other-libname",
			LibVersion: "2.1",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "4", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
				{Name: "5", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}},
			},
		},
	})

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-out:
			case <-stop:
				return
			}
		}
	}()
	defer close(stop)

	rcv.Export(context.Background(), req)

	calls := stats.CountCalls
	assert.Equal(5, len(calls))
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 3, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 1, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.payload", Value: 1, Tags: []string{"endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
}

func TestOTLPReceiverGRPC(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPReceiverGRPC(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPReceiverGRPC(true, t)
	})
}

func testOTLPReceiverGRPC(enableReceiveResourceSpansV2 bool, t *testing.T) {
	t.Helper()
	t.Run("Start/nil", func(t *testing.T) {
		cfg := NewTestConfig(t)
		if !enableReceiveResourceSpansV2 {
			cfg.Features["disable_receive_resource_spans_v2"] = struct{}{}
		}
		o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.grpcsrv)
	})

	t.Run("Start/grpc", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		cfg := NewTestConfig(t)
		if !enableReceiveResourceSpansV2 {
			cfg.Features["disable_receive_resource_spans_v2"] = struct{}{}
		}
		cfg.OTLPReceiver = &config.OTLP{
			BindHost: "localhost",
			GRPCPort: port,
		}
		o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		o.Start()
		defer o.Stop()
		assert := assert.New(t)
		assert.NotNil(o.grpcsrv)
		svc, ok := o.grpcsrv.GetServiceInfo()["opentelemetry.proto.collector.trace.v1.TraceService"]
		assert.True(ok)
		assert.Equal("opentelemetry/proto/collector/trace/v1/trace_service.proto", svc.Metadata)
		assert.Equal("Export", svc.Methods[0].Name)
	})

}

func TestTagsFromHeaders(t *testing.T) {
	out := tagsFromHeaders(http.Header(map[string][]string{
		header.Lang:                  {"go"},
		header.LangVersion:           {"1.14"},
		header.LangInterpreter:       {"x"},
		header.LangInterpreterVendor: {"y"},
	}))
	assert.Equal(t, []string{"endpoint_version:opentelemetry_grpc_v1", "lang:go", "lang_version:1.14", "interpreter:x", "lang_vendor:y"}, out)
}
