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
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"

	"github.com/DataDog/datadog-go/v5/statsd"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// Ensure spanModifier implements both the pb and idx modifier interfaces.
var (
	_ agent.SpanModifier   = (*spanModifier)(nil)
	_ agent.SpanModifierV1 = (*spanModifier)(nil)
)

func TestSpanModifierSetsOrigin(t *testing.T) {
	sm := &spanModifier{ddOrigin: "cloudrun"}

	t.Run("sets span tag and chunk origin when empty", func(t *testing.T) {
		span := &pb.Span{}
		chunk := &pb.TraceChunk{Spans: []*pb.Span{span}}

		sm.ModifySpan(chunk, span)

		assert.Equal(t, "cloudrun", span.Meta[ddOriginTagName])
		assert.Equal(t, "cloudrun", chunk.Origin, "chunk origin should be populated when empty")
	})

	t.Run("does not overwrite an existing span tag or chunk origin", func(t *testing.T) {
		span := &pb.Span{Meta: map[string]string{ddOriginTagName: "existing-span"}}
		chunk := &pb.TraceChunk{Origin: "existing-chunk", Spans: []*pb.Span{span}}

		sm.ModifySpan(chunk, span)

		assert.Equal(t, "existing-span", span.Meta[ddOriginTagName])
		assert.Equal(t, "existing-chunk", chunk.Origin, "chunk origin should not be overwritten")
	})

	t.Run("tolerates a nil chunk", func(t *testing.T) {
		span := &pb.Span{}
		assert.NotPanics(t, func() { sm.ModifySpan(nil, span) })
		assert.Equal(t, "cloudrun", span.Meta[ddOriginTagName])
	})
}

func TestSpanModifierV1SetsOrigin(t *testing.T) {
	sm := &spanModifier{ddOrigin: "cloudrun"}

	t.Run("sets span attribute and chunk origin when empty", func(t *testing.T) {
		strings := idx.NewStringTable()
		span := idx.NewInternalSpan(strings, &idx.Span{})
		chunk := idx.NewInternalTraceChunk(strings, 0, "", nil, []*idx.InternalSpan{span}, false, make([]byte, 16), 0)

		sm.ModifySpanV1(chunk, span)

		got, ok := span.GetAttributeAsString(ddOriginTagName)
		assert.True(t, ok)
		assert.Equal(t, "cloudrun", got)
		assert.Equal(t, "cloudrun", chunk.Origin(), "chunk origin should be populated when empty")
	})

	t.Run("does not overwrite an existing span attribute or chunk origin", func(t *testing.T) {
		strings := idx.NewStringTable()
		span := idx.NewInternalSpan(strings, &idx.Span{})
		span.SetStringAttribute(ddOriginTagName, "existing-span")
		chunk := idx.NewInternalTraceChunk(strings, 0, "existing-chunk", nil, []*idx.InternalSpan{span}, false, make([]byte, 16), 0)

		sm.ModifySpanV1(chunk, span)

		got, _ := span.GetAttributeAsString(ddOriginTagName)
		assert.Equal(t, "existing-span", got)
		assert.Equal(t, "existing-chunk", chunk.Origin(), "chunk origin should not be overwritten")
	})

	t.Run("tolerates a nil chunk", func(t *testing.T) {
		strings := idx.NewStringTable()
		span := idx.NewInternalSpan(strings, &idx.Span{})
		assert.NotPanics(t, func() { sm.ModifySpanV1(nil, span) })
		got, ok := span.GetAttributeAsString(ddOriginTagName)
		assert.True(t, ok)
		assert.Equal(t, "cloudrun", got)
	})
}

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
