// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package trace

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"

	"github.com/DataDog/datadog-go/v5/statsd"
)

type mockTraceWriter struct {
	mu       sync.Mutex
	payloads []*writer.SampledChunks
}

func (m *mockTraceWriter) Stop() {
	//TODO implement me
	panic("not implemented")
}

func (m *mockTraceWriter) WriteChunks(pkg *writer.SampledChunks) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payloads = append(m.payloads, pkg)
}

func (m *mockTraceWriter) FlushSync() error {
	//TODO implement me
	panic("not implemented")
}

func TestServerlessServiceRewrite(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{
		"service": "myTestService",
	}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.SpanModifier = &spanModifier{
		tags: cfg.GlobalTags,
	}
	agnt.TraceWriter = &mockTraceWriter{}
	defer cancel()

	tc := testutil.RandomTraceChunk(1, 1)
	tc.Priority = 1 // ensure trace is never sampled out
	tp := testutil.TracerPayloadWithChunk(tc)
	tp.Chunks[0].Spans[0].Service = "aws.lambda"
	agnt.Process(&api.Payload{
		TracerPayload: tp,
		Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
	})
	payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
	assert.NotEmpty(t, payloads, "no payloads were written")
	span := payloads[0].TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "myTestService", span.Service)
}

func TestInferredSpanFunctionTagFiltering(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{"some": "tag", "function_arn": "arn:aws:foo:bar:baz"}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	agnt.SpanModifier = &spanModifier{
		tags: cfg.GlobalTags,
	}
	agnt.TraceWriter = &mockTraceWriter{}
	defer cancel()

	tc := testutil.RandomTraceChunk(2, 1)
	tc.Priority = 1 // ensure trace is never sampled out
	tp := testutil.TracerPayloadWithChunk(tc)
	tp.Chunks[0].Spans[0].Meta["_inferred_span.tag_source"] = "self"
	tp.Chunks[0].Spans[1].Meta["_dd_origin"] = "lambda"
	agnt.Process(&api.Payload{
		TracerPayload: tp,
		Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
	})
	payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
	assert.NotEmpty(t, payloads, "no payloads were written")
	tp = payloads[0].TracerPayload

	_, lambdaSpanHasGlobalTags := tp.Chunks[0].Spans[1].GetMeta()["function_arn"]
	assert.True(t, lambdaSpanHasGlobalTags, "The regular span should get global tags")
	_, tagOriginSelfSpanHasGlobalTags := tp.Chunks[0].Spans[0].GetMeta()["function_arn"]
	assert.False(t, tagOriginSelfSpanHasGlobalTags, "A span with meta._inferred_span.tag_origin = self should not get global tags")
}

func TestSpanModifierAddsOriginToAllSpans(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{"some": "tag", "_dd.origin": "lambda"}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testOriginTags := func(withModifier bool) {
		agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
		if withModifier {
			agnt.SpanModifier = &spanModifier{tags: cfg.GlobalTags, ddOrigin: getDDOrigin()}
		}
		agnt.TraceWriter = &mockTraceWriter{}
		tc := testutil.RandomTraceChunk(2, 1)
		tc.Priority = 1 // ensure trace is never sampled out
		tp := testutil.TracerPayloadWithChunk(tc)

		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})
		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		tp = payloads[0].TracerPayload

		for _, chunk := range tp.Chunks {
			if chunk.Origin != "lambda" {
				t.Errorf("chunk should have Origin=lambda but has %#v", chunk.Origin)
			}
			for _, span := range chunk.Spans {
				tags := span.GetMeta()
				originVal, ok := tags["_dd.origin"]
				if withModifier != ok {
					t.Errorf("unexpected span tags, should have _dd.origin tag %#v: tags=%#v",
						withModifier, tags)
				}
				if withModifier && originVal != "lambda" {
					t.Errorf("got the wrong origin tag value: %#v", originVal)
				}
			}
		}
	}

	testOriginTags(true)
	testOriginTags(false)
}

func TestSpanModifierDetectsCloudService(t *testing.T) {
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testOriginTags := func(withModifier bool, expectedOrigin string) {
		agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
		if withModifier {
			agnt.SpanModifier = &spanModifier{ddOrigin: getDDOrigin()}
		}
		agnt.TraceWriter = &mockTraceWriter{}
		tc := testutil.RandomTraceChunk(2, 1)
		tc.Priority = 1 // ensure trace is never sampled out
		tp := testutil.TracerPayloadWithChunk(tc)

		agnt.Process(&api.Payload{
			TracerPayload: tp,
			Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
		})

		payloads := agnt.TraceWriter.(*mockTraceWriter).payloads
		assert.NotEmpty(t, payloads, "no payloads were written")
		tp = payloads[0].TracerPayload

		for _, chunk := range tp.Chunks {
			if chunk.Origin != expectedOrigin {
				t.Errorf("chunk should have Origin=%s but has %#v", expectedOrigin, chunk.Origin)
			}
			for _, span := range chunk.Spans {
				tags := span.GetMeta()
				originVal, ok := tags["_dd.origin"]
				if withModifier != ok {
					t.Errorf("unexpected span tags, should have _dd.origin tag %#v: tags=%#v",
						withModifier, tags)
				}
				if withModifier && originVal != expectedOrigin {
					t.Errorf("got the wrong origin tag value: %#v", originVal)
					t.Errorf("expected: %#v", expectedOrigin)
				}
			}
		}
	}

	// Test with and without the span modifier between different cloud services
	cloudServiceToEnvVar := map[string]string{
		"cloudrun":     cloudservice.ServiceNameEnvVar,
		"containerapp": cloudservice.ContainerAppNameEnvVar,
		"appservice":   cloudservice.WebsiteStack,
		"lambda":       functionNameEnvVar}
	for origin, cloudServiceEnvVar := range cloudServiceToEnvVar {
		// Set the appropriate environment variable to simulate a cloud service
		t.Setenv(cloudServiceEnvVar, "myService")
		cfg.GlobalTags = map[string]string{"some": "tag", "_dd.origin": origin}
		testOriginTags(true, origin)
		testOriginTags(false, origin)
		os.Unsetenv(cloudServiceEnvVar)
	}
}

func TestLambdaSpanChan(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{
		"service": "myTestService",
	}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	lambdaSpanChan := make(chan *pb.Span)
	agnt.SpanModifier = &spanModifier{
		tags:           cfg.GlobalTags,
		lambdaSpanChan: lambdaSpanChan,
	}
	defer cancel()

	tc := testutil.RandomTraceChunk(1, 1)
	tc.Priority = 1 // ensure trace is never sampled out
	tp := testutil.TracerPayloadWithChunk(tc)
	tp.Chunks[0].Spans[0].Service = "aws.lambda"
	tp.Chunks[0].Spans[0].Name = "aws.lambda"
	go agnt.Process(&api.Payload{
		TracerPayload: tp,
		Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
	})
	timeout := time.After(2 * time.Second)
	var span *pb.Span
	select {
	case ss := <-lambdaSpanChan:
		span = ss
	case <-timeout:
		t.Fatal("timed out")
	}
	assert.Equal(t, "myTestService", span.Service)
}

func TestLambdaSpanChanWithInvalidSpan(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{
		"service": "myTestService",
	}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := agent.NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	lambdaSpanChan := make(chan *pb.Span)
	agnt.SpanModifier = &spanModifier{
		tags:           cfg.GlobalTags,
		lambdaSpanChan: lambdaSpanChan,
	}
	defer cancel()

	tc := testutil.RandomTraceChunk(1, 1)
	tc.Priority = 1 // ensure trace is never sampled out
	tp := testutil.TracerPayloadWithChunk(tc)
	tp.Chunks[0].Spans[0].Service = "aws.lambda"
	tp.Chunks[0].Spans[0].Name = "not.aws.lambda"
	go agnt.Process(&api.Payload{
		TracerPayload: tp,
		Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
	})
	timeout := time.After(time.Millisecond)
	timedOut := false
	select {
	case ss := <-lambdaSpanChan:
		t.Fatalf("received a non-lambda named span, %v", ss)
	case <-timeout:
		timedOut = true
	}
	assert.Equal(t, true, timedOut)
}
