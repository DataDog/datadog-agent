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

func (m *mockTraceWriter) UpdateAPIKey(_, _ string) {}

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
		"appservice":   cloudservice.WebsiteStack}
	for origin, cloudServiceEnvVar := range cloudServiceToEnvVar {
		// Set the appropriate environment variable to simulate a cloud service
		t.Setenv(cloudServiceEnvVar, "myService")
		cfg.GlobalTags = map[string]string{"some": "tag", "_dd.origin": origin}
		testOriginTags(true, origin)
		testOriginTags(false, origin)
		os.Unsetenv(cloudServiceEnvVar)
	}
}

// TestSpanModifierModifySpanBeforeSetTags verifies that ModifySpan only
// applies the _dd.origin tag when SetTags has never been called, since
// spanModifier.tags starts as an unset atomic.Pointer.
func TestSpanModifierModifySpanBeforeSetTags(t *testing.T) {
	sm := &spanModifier{ddOrigin: "lambda"}
	span := &pb.Span{Meta: map[string]string{}}

	sm.ModifySpan(&pb.TraceChunk{}, span)

	assert.Equal(t, "lambda", span.Meta[ddOriginTagName])
	assert.Len(t, span.Meta, 1)
}

// TestSpanModifierModifySpanAppliesTagsSetDynamically verifies that tags
// applied via SetTags after construction are picked up by ModifySpan, the
// mechanism MicroVM uses to deliver the lambda_microvm_id tag once it becomes
// known at /run time.
func TestSpanModifierModifySpanAppliesTagsSetDynamically(t *testing.T) {
	sm := &spanModifier{ddOrigin: "lambda"}
	sm.SetTags(map[string]string{"lambda_microvm_id": "vm-123"})

	span := &pb.Span{Meta: map[string]string{}}
	sm.ModifySpan(&pb.TraceChunk{}, span)

	assert.Equal(t, "lambda", span.Meta[ddOriginTagName])
	assert.Equal(t, "vm-123", span.Meta["lambda_microvm_id"])
}

// TestSpanModifierModifySpanPreservesExistingOrigin verifies that ModifySpan
// does not overwrite a span's existing _dd.origin when the dynamically-set
// tags also contain _dd.origin. Every CloudService.GetTags() sets _dd.origin
// (e.g. MicroVM sets "lambdamicrovm"), and that value flows into the tags
// applied here via SetTags/UpdateRuntimeTags — so without this guard, every
// span would have a tracer-supplied origin (e.g. "rum") silently replaced.
func TestSpanModifierModifySpanPreservesExistingOrigin(t *testing.T) {
	sm := &spanModifier{ddOrigin: "lambda"}
	sm.SetTags(map[string]string{ddOriginTagName: "lambdamicrovm", "lambda_microvm_id": "vm-123"})

	span := &pb.Span{Meta: map[string]string{ddOriginTagName: "rum"}}
	sm.ModifySpan(&pb.TraceChunk{}, span)

	assert.Equal(t, "rum", span.Meta[ddOriginTagName], "must not overwrite a tracer-supplied origin")
	assert.Equal(t, "vm-123", span.Meta["lambda_microvm_id"])
}

// TestSpanModifierModifySpanReflectsLatestSetTags verifies that a later
// SetTags call replaces the tag set used by subsequent ModifySpan calls.
func TestSpanModifierModifySpanReflectsLatestSetTags(t *testing.T) {
	sm := &spanModifier{ddOrigin: "lambda"}
	sm.SetTags(map[string]string{"lambda_microvm_id": "vm-1"})
	sm.SetTags(map[string]string{"lambda_microvm_id": "vm-2"})

	span := &pb.Span{Meta: map[string]string{}}
	sm.ModifySpan(&pb.TraceChunk{}, span)

	assert.Equal(t, "vm-2", span.Meta["lambda_microvm_id"])
}

// TestSpanModifierSetTagsConcurrentWithModifySpan exercises SetTags and
// ModifySpan concurrently under the race detector. This is a regression test
// for the data race Codex flagged on PR #53036: MicroVM's /run hook calls
// SetTags from a goroutine that runs concurrently with the trace agent's
// span-processing loop, which calls ModifySpan on every span.
func TestSpanModifierSetTagsConcurrentWithModifySpan(t *testing.T) {
	sm := &spanModifier{ddOrigin: "lambda"}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			sm.SetTags(map[string]string{"lambda_microvm_id": "vm-1"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			span := &pb.Span{Meta: map[string]string{}}
			sm.ModifySpan(&pb.TraceChunk{}, span)
		}
	}()
	wg.Wait()

	assert.Equal(t, map[string]string{"lambda_microvm_id": "vm-1"}, *sm.tags.Load())
}
