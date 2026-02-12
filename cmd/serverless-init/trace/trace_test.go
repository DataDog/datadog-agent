// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
)

// mockProcessor is a mock implementation of the Processor interface for testing
type mockProcessor struct {
	processCalled bool
	lastPayload   *api.PayloadV1
}

func (m *mockProcessor) ProcessV1(p *api.PayloadV1) {
	m.processCalled = true
	m.lastPayload = p
}

func TestInitChunk(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{
		"key1": "value1",
		"env":  "test",
	}

	chunk := InitChunk("test-service", "test.operation", "test-resource", "web", startTime, tags)

	assert.NotNil(t, chunk)
	assert.Len(t, chunk.Spans, 1)

	span := chunk.Spans[0]
	assert.Equal(t, "test-service", span.Service())
	assert.Equal(t, "test.operation", span.Name())
	assert.Equal(t, "test-resource", span.Resource())
	assert.Equal(t, "web", span.Type())
	assert.Equal(t, uint64(startTime), span.Start())
	assert.NotZero(t, chunk.LegacyTraceID())
	assert.NotZero(t, span.SpanID())
	assert.Equal(t, uint64(0), span.ParentID())

	// Check tags - some have been promoted out of field (env) so we check those separately
	val, found := span.GetAttributeAsString("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", val)
	assert.Equal(t, "test", span.Env())
}

func TestInitChunkGeneratesUniqueIDs(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{}

	chunk1 := InitChunk("service", "name", "resource", "type", startTime, tags)
	chunk2 := InitChunk("service", "name", "resource", "type", startTime, tags)

	// TraceIDs and SpanIDs should be different
	assert.NotEqual(t, chunk1.LegacyTraceID(), chunk2.LegacyTraceID())
	assert.NotEqual(t, chunk1.Spans[0].SpanID(), chunk2.Spans[0].SpanID())
}

func TestSubmitSpanWithNilChunk(t *testing.T) {
	mockAgent := &mockProcessor{}

	// Should not panic and should not call ProcessV1
	SubmitSpan(nil, "test-origin", mockAgent)

	assert.False(t, mockAgent.processCalled)
}

func TestSubmitSpanWithNilTraceAgent(_ *testing.T) {
	chunk := InitChunk("test-service", "test.operation", "test-resource", "web", time.Now().UnixNano(), nil)

	// Should not panic
	SubmitSpan(chunk, "test-origin", nil)
}

func TestSubmitSpanWithValidProcessor(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{"env": "test"}
	chunk := InitChunk("test-service", "test.operation", "test-resource", "web", startTime, tags)
	chunk.Spans[0].SetDuration(1000000) // 1ms

	mockAgent := &mockProcessor{}

	SubmitSpan(chunk, "test-origin", mockAgent)

	assert.True(t, mockAgent.processCalled)
	assert.NotNil(t, mockAgent.lastPayload)
	assert.NotNil(t, mockAgent.lastPayload.TracerPayload)
	assert.Len(t, mockAgent.lastPayload.TracerPayload.Chunks, 1)

	resultChunk := mockAgent.lastPayload.TracerPayload.Chunks[0]
	assert.Equal(t, "test-origin", resultChunk.Origin())
	assert.Equal(t, int32(1), resultChunk.Priority)
	assert.Len(t, resultChunk.Spans, 1)
	assert.Equal(t, chunk.Spans[0], resultChunk.Spans[0])
}
