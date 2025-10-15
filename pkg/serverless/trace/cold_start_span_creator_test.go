// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-go/v5/statsd"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"

	comptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl-noop"
)

func TestColdStartSpanCreatorCreateValid(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create noop telemetry components for testing
	noopTelem1 := comptelemetry.NewComponent()
	receiverTelem1 := info.NewReceiverTelemetry(noopTelem1)
	statsWriterTelem1 := writer.NewStatsWriterTelemetry(noopTelem1)
	traceWriterTelem1 := writer.NewTraceWriterTelemetry(noopTelem1)

	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent(), receiverTelem1, statsWriterTelem1, traceWriterTelem1)
	agnt.TraceWriter = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}

	coldStartDuration := 50.0 // Given in millis

	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLog.LambdaInitMetric)
	stopChan := make(chan struct{})
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	initReportStartTime := time.Now().Add(-1 * time.Second)
	lambdaInitMetricDuration := &serverlessLog.LambdaInitMetric{
		InitDurationTelemetry: coldStartDuration,
	}
	lambdaInitMetricStartTime := &serverlessLog.LambdaInitMetric{
		InitStartTime: initReportStartTime,
	}
	coldStartSpanCreator := &ColdStartSpanCreator{
		TraceAgent:           traceAgent,
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		ColdStartSpanId:      coldStartSpanId,
		StopChan:             stopChan,
	}

	coldStartSpanCreator.Run()
	defer coldStartSpanCreator.Stop()

	now := time.Now().UnixNano()
	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda",
		Start:    now,
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500000000,
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime

	span := firstWrittenSpan(t, agnt.TraceWriter.(*mockTraceWriter))

	assert.Equal(t, "aws.lambda", span.Service)
	assert.Equal(t, "aws.lambda.cold_start", span.Name)
	assert.Equal(t, initReportStartTime.UnixNano(), span.Start)
	assert.Equal(t, int64(coldStartDuration*1000000), span.Duration)
}

func TestColdStartSpanCreatorCreateValidNoOverlap(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create noop telemetry components for testing
	noopTelem2 := comptelemetry.NewComponent()
	receiverTelem2 := info.NewReceiverTelemetry(noopTelem2)
	statsWriterTelem2 := writer.NewStatsWriterTelemetry(noopTelem2)
	traceWriterTelem2 := writer.NewTraceWriterTelemetry(noopTelem2)

	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent(), receiverTelem2, statsWriterTelem2, traceWriterTelem2)
	agnt.TraceWriter = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}

	coldStartDuration := 50.0 // Given in millis

	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLog.LambdaInitMetric)
	initReportStartTime := time.Now().Add(10 * time.Second)
	lambdaInitMetricDuration := &serverlessLog.LambdaInitMetric{
		InitDurationTelemetry: coldStartDuration,
	}
	lambdaInitMetricStartTime := &serverlessLog.LambdaInitMetric{
		InitStartTime: initReportStartTime,
	}
	stopChan := make(chan struct{})
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	coldStartSpanCreator := &ColdStartSpanCreator{
		TraceAgent:           traceAgent,
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		ColdStartSpanId:      coldStartSpanId,
		StopChan:             stopChan,
	}

	coldStartSpanCreator.Run()
	defer coldStartSpanCreator.Stop()

	now := time.Now().UnixNano()
	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda",
		Start:    now,
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500000000,
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	span := firstWrittenSpan(t, agnt.TraceWriter.(*mockTraceWriter))
	assert.Equal(t, "aws.lambda", span.Service)
	assert.Equal(t, "aws.lambda.cold_start", span.Name)
	assert.Equal(t, now-int64(coldStartDuration*1000000), span.Start)
	assert.Equal(t, int64(coldStartDuration*1000000), span.Duration)
}

func TestColdStartSpanCreatorCreateDuplicate(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create noop telemetry components for testing
	noopTelem3 := comptelemetry.NewComponent()
	receiverTelem3 := info.NewReceiverTelemetry(noopTelem3)
	statsWriterTelem3 := writer.NewStatsWriterTelemetry(noopTelem3)
	traceWriterTelem3 := writer.NewTraceWriterTelemetry(noopTelem3)

	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent(), receiverTelem3, statsWriterTelem3, traceWriterTelem3)
	agnt.TraceWriter = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}
	coldStartDuration := 50.0 // Given in millis
	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLog.LambdaInitMetric)
	initReportStartTime := time.Now().Add(-1 * time.Second)
	lambdaInitMetricDuration := &serverlessLog.LambdaInitMetric{
		InitDurationTelemetry: coldStartDuration,
	}
	lambdaInitMetricStartTime := &serverlessLog.LambdaInitMetric{
		InitStartTime: initReportStartTime,
	}
	stopChan := make(chan struct{})
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	coldStartSpanCreator := &ColdStartSpanCreator{
		TraceAgent:           traceAgent,
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		ColdStartSpanId:      coldStartSpanId,
		StopChan:             stopChan,
	}

	coldStartSpanCreator.Run()
	defer coldStartSpanCreator.Stop()

	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda.cold_start",
		Start:    time.Now().Unix(),
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500,
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	<-time.After(time.Millisecond)
	payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
	assert.Empty(t, payloads, "created a coldstart span when we should have passed")
}

func TestColdStartSpanCreatorNotColdStart(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create noop telemetry components for testing
	noopTelem4 := comptelemetry.NewComponent()
	receiverTelem4 := info.NewReceiverTelemetry(noopTelem4)
	statsWriterTelem4 := writer.NewStatsWriterTelemetry(noopTelem4)
	traceWriterTelem4 := writer.NewTraceWriterTelemetry(noopTelem4)

	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent(), receiverTelem4, statsWriterTelem4, traceWriterTelem4)
	agnt.TraceWriter = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}
	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLog.LambdaInitMetric)
	stopChan := make(chan struct{})
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	coldStartSpanCreator := &ColdStartSpanCreator{
		TraceAgent:           traceAgent,
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		ColdStartSpanId:      coldStartSpanId,
		StopChan:             stopChan,
	}

	coldStartSpanCreator.Run()
	defer coldStartSpanCreator.Stop()

	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda.my-function",
		Start:    time.Now().Unix(),
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500,
	}
	lambdaSpanChan <- lambdaSpan
	// Don't write to lambdaInitMetricChan, as this is not a cold start
	<-time.After(time.Millisecond)
	payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
	assert.Empty(t, payloads, "created a coldstart span when we should have passed")
}

func TestColdStartSpanCreatorColdStartExists(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create noop telemetry components for testing
	noopTelem5 := comptelemetry.NewComponent()
	receiverTelem5 := info.NewReceiverTelemetry(noopTelem5)
	statsWriterTelem5 := writer.NewStatsWriterTelemetry(noopTelem5)
	traceWriterTelem5 := writer.NewTraceWriterTelemetry(noopTelem5)

	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent(), receiverTelem5, statsWriterTelem5, traceWriterTelem5)
	agnt.TraceWriter = &mockTraceWriter{}

	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}
	coldStartDuration := 50.0 // Given in millis
	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLog.LambdaInitMetric)
	initReportStartTime := time.Now().Add(-1 * time.Second)
	lambdaInitMetricDuration := &serverlessLog.LambdaInitMetric{
		InitDurationTelemetry: coldStartDuration,
	}
	lambdaInitMetricStartTime := &serverlessLog.LambdaInitMetric{
		InitStartTime: initReportStartTime,
	}
	stopChan := make(chan struct{})
	coldStartSpanID := random.Random.Uint64()
	coldStartSpanCreator := &ColdStartSpanCreator{
		TraceAgent:           traceAgent,
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		ColdStartSpanId:      coldStartSpanID,
		StopChan:             stopChan,
		ColdStartRequestID:   "test",
	}

	coldStartSpanCreator.Run()
	defer coldStartSpanCreator.Stop()

	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda",
		Start:    time.Now().Unix(),
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500,
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	<-time.After(time.Millisecond)
	payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
	assert.Empty(t, payloads, "created a coldstart span when we should have passed")
}

func TestColdStartSpanCreatorCreateValidProvisionedConcurrency(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create noop telemetry components for testing
	noopTelem6 := comptelemetry.NewComponent()
	receiverTelem6 := info.NewReceiverTelemetry(noopTelem6)
	statsWriterTelem6 := writer.NewStatsWriterTelemetry(noopTelem6)
	traceWriterTelem6 := writer.NewTraceWriterTelemetry(noopTelem6)

	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent(), receiverTelem6, statsWriterTelem6, traceWriterTelem6)
	agnt.TraceWriter = &mockTraceWriter{}

	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}

	coldStartDuration := 50.0 // Given in millis

	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLog.LambdaInitMetric)
	stopChan := make(chan struct{})
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	initReportStartTime := time.Now().Add(-10 * time.Minute)
	lambdaInitMetricDuration := &serverlessLog.LambdaInitMetric{
		InitDurationTelemetry: coldStartDuration,
	}
	lambdaInitMetricStartTime := &serverlessLog.LambdaInitMetric{
		InitStartTime: initReportStartTime,
	}
	coldStartSpanCreator := &ColdStartSpanCreator{
		TraceAgent:           traceAgent,
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		ColdStartSpanId:      coldStartSpanId,
		StopChan:             stopChan,
	}

	coldStartSpanCreator.Run()
	defer coldStartSpanCreator.Stop()

	now := time.Now().UnixNano()
	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda",
		Start:    now,
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500000000,
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	lambdaInitMetricChan <- lambdaInitMetricDuration

	span := firstWrittenSpan(t, agnt.TraceWriter.(*mockTraceWriter))

	assert.Equal(t, "aws.lambda", span.Service)
	assert.Equal(t, "aws.lambda.cold_start", span.Name)
	assert.Equal(t, initReportStartTime.UnixNano(), span.Start)
	assert.Equal(t, int64(coldStartDuration*1000000), span.Duration)
}

func firstWrittenSpan(t *testing.T, tw *mockTraceWriter) *pb.Span {
	timeout := time.After(2 * time.Second)
	for {
		select {
		default:
			tw.mu.Lock()
			payloads := tw.payloads
			if len(payloads) > 0 {
				return payloads[0].TracerPayload.Chunks[0].Spans[0]
			}
			tw.mu.Unlock()
		case <-timeout:
			t.Fatal("timed out")
		}
	}
}
