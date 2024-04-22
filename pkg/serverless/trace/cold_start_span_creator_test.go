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

	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

func TestColdStartSpanCreatorCreateValid(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
	traceAgent := &ServerlessTraceAgent{
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

	timeout := time.After(2 * time.Second)
	var span *pb.Span
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		span = ss.TracerPayload.Chunks[0].Spans[0]
	case <-timeout:
		t.Fatal("timed out")
	}
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
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
	traceAgent := &ServerlessTraceAgent{
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
	timeout := time.After(2 * time.Second)
	var span *pb.Span
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		span = ss.TracerPayload.Chunks[0].Spans[0]
	case <-timeout:
		t.Fatal("timed out")
	}
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
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
	traceAgent := &ServerlessTraceAgent{
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
	timeout := time.After(time.Millisecond)
	timedOut := false
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		t.Fatalf("created a coldstart span when we should have passed, %v", ss)
	case <-timeout:
		timedOut = true
	}
	assert.Equal(t, true, timedOut)
}

func TestColdStartSpanCreatorNotColdStart(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
	traceAgent := &ServerlessTraceAgent{
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

	timeout := time.After(time.Millisecond)
	timedOut := false
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		t.Fatalf("created a coldstart span when we should have passed, %v", ss)
	case <-timeout:
		timedOut = true
	}
	assert.Equal(t, true, timedOut)
}

func TestColdStartSpanCreatorColdStartExists(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
	traceAgent := &ServerlessTraceAgent{
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
	timeout := time.After(time.Millisecond)
	timedOut := false
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		t.Fatalf("created a coldstart span when we should have passed, %v", ss)
	case <-timeout:
		timedOut = true
	}
	assert.Equal(t, true, timedOut)
}

func TestColdStartSpanCreatorCreateValidProvisionedConcurrency(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
	traceAgent := &ServerlessTraceAgent{
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

	timeout := time.After(2 * time.Second)
	var span *pb.Span
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		span = ss.TracerPayload.Chunks[0].Spans[0]
	case <-timeout:
		t.Fatal("timed out")
	}
	assert.Equal(t, "aws.lambda", span.Service)
	assert.Equal(t, "aws.lambda.cold_start", span.Name)
	assert.Equal(t, initReportStartTime.UnixNano(), span.Start)
	assert.Equal(t, int64(coldStartDuration*1000000), span.Duration)
}
