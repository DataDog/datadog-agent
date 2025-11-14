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

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-go/v5/statsd"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
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
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.TraceWriterV1 = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}

	coldStartDuration := 50.0 // Given in millis

	lambdaSpanChan := make(chan *LambdaSpan)
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
	st := idx.NewStringTable()
	lambdaSpan := &LambdaSpan{
		TraceID: make([]byte, 16),
		Span: idx.NewInternalSpan(st, &idx.Span{
			ServiceRef:  st.Add("aws.lambda"),
			NameRef:     st.Add("aws.lambda"),
			ResourceRef: st.Add(functionName),
			SpanID:      random.Random.Uint64(),
			ParentID:    random.Random.Uint64(),
			Start:       uint64(now),
			Duration:    uint64(500000000),
		}),
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime

	span := firstWrittenSpan(t, agnt.TraceWriterV1.(*mockTraceWriter))

	assert.Equal(t, "aws.lambda", span.Service())
	assert.Equal(t, "aws.lambda.cold_start", span.Name())
	assert.Equal(t, uint64(initReportStartTime.UnixNano()), span.Start())
	assert.Equal(t, uint64(coldStartDuration*1000000), span.Duration())
}

func TestColdStartSpanCreatorCreateValidNoOverlap(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.TraceWriterV1 = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}

	coldStartDuration := 50.0 // Given in millis

	lambdaSpanChan := make(chan *LambdaSpan)
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
	st := idx.NewStringTable()
	lambdaSpan := &LambdaSpan{
		TraceID: make([]byte, 16),
		Span: idx.NewInternalSpan(st, &idx.Span{
			ServiceRef:  st.Add("aws.lambda"),
			NameRef:     st.Add("aws.lambda"),
			ResourceRef: st.Add(functionName),
			SpanID:      random.Random.Uint64(),
			ParentID:    random.Random.Uint64(),
			Start:       uint64(now),
			Duration:    uint64(500000000),
		}),
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	span := firstWrittenSpan(t, agnt.TraceWriterV1.(*mockTraceWriter))
	assert.Equal(t, "aws.lambda", span.Service())
	assert.Equal(t, "aws.lambda.cold_start", span.Name())
	assert.Equal(t, uint64(now-int64(coldStartDuration*1000000)), span.Start())
	assert.Equal(t, uint64(coldStartDuration*1000000), span.Duration())
}

func TestColdStartSpanCreatorCreateDuplicate(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.TraceWriterV1 = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}
	coldStartDuration := 50.0 // Given in millis
	lambdaSpanChan := make(chan *LambdaSpan)
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

	now := time.Now().UnixNano()
	st := idx.NewStringTable()
	lambdaSpan := &LambdaSpan{
		TraceID: make([]byte, 16),
		Span: idx.NewInternalSpan(st, &idx.Span{
			ServiceRef:  st.Add("aws.lambda"),
			NameRef:     st.Add("aws.lambda.cold_start"),
			ResourceRef: st.Add(functionName),
			SpanID:      random.Random.Uint64(),
			ParentID:    random.Random.Uint64(),
			Start:       uint64(now),
			Duration:    uint64(500),
		}),
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	<-time.After(time.Millisecond)
	payloads := agnt.TraceWriterV1.(*mockTraceWriter).payloadsV1
	assert.Empty(t, payloads, "created a coldstart span when we should have passed")
}

func TestColdStartSpanCreatorNotColdStart(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.TraceWriterV1 = &mockTraceWriter{}
	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}
	lambdaSpanChan := make(chan *LambdaSpan)
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

	now := time.Now().UnixNano()
	st := idx.NewStringTable()
	lambdaSpan := &LambdaSpan{
		TraceID: make([]byte, 16),
		Span: idx.NewInternalSpan(st, &idx.Span{
			ServiceRef:  st.Add("aws.lambda"),
			NameRef:     st.Add("aws.lambda.my-function"),
			ResourceRef: st.Add(functionName),
			SpanID:      random.Random.Uint64(),
			ParentID:    random.Random.Uint64(),
			Start:       uint64(now),
			Duration:    uint64(500),
		}),
	}
	lambdaSpanChan <- lambdaSpan
	// Don't write to lambdaInitMetricChan, as this is not a cold start
	<-time.After(time.Millisecond)
	payloads := agnt.TraceWriterV1.(*mockTraceWriter).payloadsV1
	assert.Empty(t, payloads, "created a coldstart span when we should have passed")
}

func TestColdStartSpanCreatorColdStartExists(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.TraceWriterV1 = &mockTraceWriter{}

	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}
	coldStartDuration := 50.0 // Given in millis
	lambdaSpanChan := make(chan *LambdaSpan)
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

	now := time.Now().UnixNano()
	st := idx.NewStringTable()
	lambdaSpan := &LambdaSpan{
		TraceID: make([]byte, 16),
		Span: idx.NewInternalSpan(st, &idx.Span{
			ServiceRef:  st.Add("aws.lambda"),
			NameRef:     st.Add("aws.lambda"),
			ResourceRef: st.Add(functionName),
			SpanID:      random.Random.Uint64(),
			ParentID:    random.Random.Uint64(),
			Start:       uint64(now),
			Duration:    uint64(500),
		}),
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricDuration
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	<-time.After(time.Millisecond)
	payloads := agnt.TraceWriterV1.(*mockTraceWriter).payloadsV1
	assert.Empty(t, payloads, "created a coldstart span when we should have passed")
}

func TestColdStartSpanCreatorCreateValidProvisionedConcurrency(t *testing.T) {
	setupTraceAgentTest(t)

	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.TraceWriterV1 = &mockTraceWriter{}

	traceAgent := &serverlessTraceAgent{
		ta: agnt,
	}

	coldStartDuration := 50.0 // Given in millis

	lambdaSpanChan := make(chan *LambdaSpan)
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
	st := idx.NewStringTable()
	lambdaSpan := &LambdaSpan{
		TraceID: make([]byte, 16),
		Span: idx.NewInternalSpan(st, &idx.Span{
			ServiceRef:  st.Add("aws.lambda"),
			NameRef:     st.Add("aws.lambda"),
			ResourceRef: st.Add(functionName),
			SpanID:      random.Random.Uint64(),
			ParentID:    random.Random.Uint64(),
			Start:       uint64(now),
			Duration:    uint64(500000000),
		}),
	}
	lambdaSpanChan <- lambdaSpan
	lambdaInitMetricChan <- lambdaInitMetricStartTime
	lambdaInitMetricChan <- lambdaInitMetricDuration

	span := firstWrittenSpan(t, agnt.TraceWriterV1.(*mockTraceWriter))

	assert.Equal(t, "aws.lambda", span.Service())
	assert.Equal(t, "aws.lambda.cold_start", span.Name())
	assert.Equal(t, uint64(initReportStartTime.UnixNano()), span.Start())
	assert.Equal(t, uint64(coldStartDuration*1000000), span.Duration())
}

func firstWrittenSpan(t *testing.T, tw *mockTraceWriter) *idx.InternalSpan {
	timeout := time.After(2 * time.Second)
	for {
		select {
		default:
			tw.mu.Lock()
			payloads := tw.payloadsV1
			if len(payloads) > 0 {
				return payloads[0].TracerPayload.Chunks[0].Spans[0]
			}
			tw.mu.Unlock()
		case <-timeout:
			t.Fatal("timed out")
		}
	}
}
