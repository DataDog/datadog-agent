// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

// mockProcessor is a mock implementation of the Processor interface for testing
type mockProcessor struct {
	processCalled bool
	lastPayload   *api.PayloadV1
}

func (m *mockProcessor) Process(p *api.PayloadV1) {
	m.processCalled = true
	m.lastPayload = p
}

func TestInitSpan(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{
		"env":     "test",
		"service": "my-service",
		"version": "1.0",
	}

	span := InitSpan("test-service", "test.operation", "test-resource", "web", startTime, tags)

	assert.NotNil(t, span)
	assert.Equal(t, "test-service", span.Service)
	assert.Equal(t, "test.operation", span.Name)
	assert.Equal(t, "test-resource", span.Resource)
	assert.Equal(t, "web", span.Type)
	assert.Equal(t, startTime, span.Start)
	assert.NotZero(t, span.TraceID)
	assert.NotZero(t, span.SpanID)
	assert.Equal(t, uint64(0), span.ParentID)
	assert.Equal(t, tags, span.Meta)
}

func TestInitSpanGeneratesUniqueIDs(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{}

	span1 := InitSpan("service", "name", "resource", "type", startTime, tags)
	span2 := InitSpan("service", "name", "resource", "type", startTime, tags)

	// TraceIDs and SpanIDs should be different
	assert.NotEqual(t, span1.TraceID, span2.TraceID)
	assert.NotEqual(t, span1.SpanID, span2.SpanID)
}

func TestSubmitSpanWithNilSpan(t *testing.T) {
	mockAgent := &mockProcessor{}

	// Should not panic and should not call Process
	SubmitSpan(nil, "test-origin", mockAgent)

	assert.False(t, mockAgent.processCalled)
}

func TestSubmitSpanWithNilTraceAgent(_ *testing.T) {
	span := InitSpan("test-service", "test.operation", "test-resource", "web", time.Now().UnixNano(), nil)

	// Should not panic
	SubmitSpan(span, "test-origin", nil)
}

func TestSubmitSpanWithValidProcessor(t *testing.T) {
	startTime := time.Now().UnixNano()
	tags := map[string]string{
		"env":      "test",
		"job_name": "my-job",
	}
	span := InitSpan("test-service", "test.operation", "test-resource", "web", startTime, tags)
	span.Duration = 1000000 // 1ms
	span.Metrics = map[string]float64{"_dd.measured": 1}

	mockAgent := &mockProcessor{}

	SubmitSpan(span, "test-origin", mockAgent)

	assert.True(t, mockAgent.processCalled)
	require.NotNil(t, mockAgent.lastPayload)
	require.NotNil(t, mockAgent.lastPayload.TracerPayload)
	require.Len(t, mockAgent.lastPayload.TracerPayload.Chunks, 1)

	chunk := mockAgent.lastPayload.TracerPayload.Chunks[0]
	assert.Equal(t, "test-origin", chunk.Origin())
	assert.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
	require.Len(t, chunk.Spans, 1)

	converted := chunk.Spans[0]
	assert.Equal(t, span.Service, converted.Service())
	assert.Equal(t, span.Name, converted.Name())
	assert.Equal(t, span.Resource, converted.Resource())
	assert.Equal(t, span.Type, converted.Type())
	assert.Equal(t, span.SpanID, converted.SpanID())
	assert.Equal(t, span.ParentID, converted.ParentID())
	assert.Equal(t, uint64(span.Start), converted.Start())
	assert.Equal(t, uint64(span.Duration), converted.Duration())

	// Non-promoted meta tags become string attributes.
	got, ok := converted.GetAttributeAsString("job_name")
	assert.True(t, ok)
	assert.Equal(t, "my-job", got)
	// Metrics become float64 attributes.
	measured, ok := converted.GetAttributeAsFloat64("_dd.measured")
	assert.True(t, ok)
	assert.Equal(t, float64(1), measured)
	// Promoted tags are lifted to span fields instead of attributes: env comes
	// back via the promoted field (GetAttributeAsString("env") also resolves to
	// it through a back-compat fallback).
	assert.Equal(t, "test", converted.Env())
	// The payload was generated in-process, not converted from a wire payload,
	// so it must not carry the conversion debug attribute.
	_, ok = converted.GetAttributeAsString("_dd.convertedv1")
	assert.False(t, ok, "inferred spans must not be tagged _dd.convertedv1")
}

// TestSubmitSpanSubmitsV1PayloadType guards the regression where SubmitSpan
// submitted the legacy api.Payload instead of the v1 payload processed by the
// default convert-traces path.
func TestSubmitSpanSubmitsV1PayloadType(t *testing.T) {
	span := InitSpan("svc", "op", "res", "web", time.Now().UnixNano(), nil)
	span.Duration = 100

	mockAgent := &mockProcessor{}
	SubmitSpan(span, "test-origin", mockAgent)

	assert.True(t, mockAgent.processCalled)
	// The v1 payload is identifiable by its string-indexed tracer payload.
	require.NotNil(t, mockAgent.lastPayload)
	require.NotNil(t, mockAgent.lastPayload.TracerPayload)
	assert.NotNil(t, mockAgent.lastPayload.TracerPayload.Strings)
}
