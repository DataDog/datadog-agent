// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp
// +build otlp

package otlp

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	// ensure trace agent is always started on a free port
	if port, err := testutil.FindTCPPort(); err == nil {
		os.Setenv("DD_RECEIVER_PORT", strconv.Itoa(port))
	}
	// ensure dogstatsd is always started on a free port
	if port, err := testutil.FindTCPPort(); err == nil {
		os.Setenv("DD_DOGSTATSD_PORT", strconv.Itoa(port))
	}
	os.Exit(m.Run())
}

func TestServerlessOTLPAgentReceivesTraces(t *testing.T) {
	assert := assert.New(t)

	// ensure internal otlp trace endpoint is always started on new port
	tracePort, err := testutil.FindTCPPort()
	assert.Nil(err)
	t.Setenv("DD_OTLP_CONFIG_TRACES_INTERNAL_PORT", strconv.Itoa(tracePort))

	// in the case where test is run without the serverless build tag, skip
	// hostname resolution
	t.Setenv("DD_HOSTNAME", "myhostname")

	grpcEndpoint, httpEndpoint := "localhost:4317", "localhost:4318"
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT", httpEndpoint)
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", grpcEndpoint)

	// setup metric agent
	config.DetectFeatures()
	metricAgent := &metrics.ServerlessMetricAgent{}
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	defer metricAgent.Stop()
	assert.NotNil(metricAgent.Demux)
	assert.True(metricAgent.IsReady())

	// setup otlp agent
	otlpAgent := NewServerlessOTLPAgent(metricAgent.Demux.Serializer())
	otlpAgent.Start()
	defer otlpAgent.Stop()
	assert.NotNil(otlpAgent.pipeline)
	assert.Nil(otlpAgent.Wait(5 * time.Second))

	// setup trace agent
	traceAgent := &trace.ServerlessTraceAgent{}
	traceAgent.Start(true, &trace.LoadConfig{Path: "./testdata/valid.yml"}, nil, 0)
	defer traceAgent.Stop()
	assert.NotNil(traceAgent.Get())
	traceChan := make(chan struct{})
	traceAgent.SetSpanModifier(func(_ *pb.TraceChunk, _ *pb.Span) {
		// indicates when trace is received
		traceChan <- struct{}{}
	})

	// test http traces
	httpClient := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(httpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	err = testServerlessOTLPAgentReceivesTraces(httpClient, traceChan)
	assert.Nil(err)

	// test grpc traces
	grpcClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(grpcEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	err = testServerlessOTLPAgentReceivesTraces(grpcClient, traceChan)
	assert.Nil(err)
}

func testServerlessOTLPAgentReceivesTraces(client otlptrace.Client, traceChan <-chan struct{}) error {
	// use opentelemetry to send spans
	ctx := context.Background()
	exporter, _ := otlptrace.New(ctx, client)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)
	_, span := tracerProvider.Tracer("abc").Start(ctx, "xyz")
	span.End()
	tracerProvider.Shutdown(ctx)

	select {
	case <-traceChan:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for span to arrive")
	}
	return nil
}
