// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFreshSpan(t *testing.T) {
	ctx := context.Background()
	_, ok := SpanFromContext(ctx)
	require.False(t, ok)

	s, ctx := StartSpanFromContext(ctx, "test")
	require.NotNil(t, s)
	s.SetResourceName("new")

	span, ok := SpanFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, s, span)

	assert.Equal(t, "test", s.span.Name)
	assert.Equal(t, "new", s.span.Resource)
	assert.Equal(t, "new", s.span.Resource)
	assert.Equal(t, "new", span.span.Resource)
}

func TestInheritence(t *testing.T) {
	ctx := context.Background()
	s, ctx := StartSpanFromContext(ctx, "test")
	require.NotNil(t, s)

	child, _ := StartSpanFromContext(ctx, "child")
	require.NotNil(t, child)

	assert.Equal(t, s.span.SpanID, child.span.ParentID)
	assert.Equal(t, s.span.TraceID, child.span.TraceID)
}

func TestStartSpanFromIDs(t *testing.T) {
	ctx := context.Background()
	traceID := "100"
	parentID := "200"

	span, ctx := StartSpanFromIDs(ctx, "ids-operation", traceID, parentID)
	require.NotNil(t, span, "Expected a span")
	require.Equal(t, uint64(100), span.span.TraceID)
	require.Equal(t, uint64(200), span.span.ParentID)

	val, ok := span.span.Metrics["_top_level"]
	require.True(t, ok)
	require.Equal(t, 1.0, val)

	spanFromCtx, ok := SpanFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, span, spanFromCtx)
}

func strPtr(s string) *string {
	return &s
}

func TestSpanFromEnv(t *testing.T) {
	randTraceID := uint64(9)
	tt := []struct {
		name             string
		envTraceID       *string
		envParentID      *string
		expectedTraceID  uint64
		expectedParentID uint64
	}{
		{
			name:             "no parent env",
			envTraceID:       strPtr("100"),
			envParentID:      nil,
			expectedTraceID:  randTraceID,
			expectedParentID: 0,
		},
		{
			name:             "no trace env",
			envTraceID:       nil,
			envParentID:      strPtr("100"),
			expectedTraceID:  randTraceID,
			expectedParentID: 0,
		},
		{
			name:             "traceID malformed",
			envTraceID:       strPtr("not-a-number"),
			envParentID:      strPtr("200"),
			expectedTraceID:  randTraceID,
			expectedParentID: 0,
		},
		{
			name:             "parentID malformed",
			envTraceID:       strPtr("100"),
			envParentID:      strPtr("not-a-number"),
			expectedTraceID:  randTraceID,
			expectedParentID: 0,
		},
		{
			name:             "inheritance",
			envTraceID:       strPtr("100"),
			envParentID:      strPtr("200"),
			expectedTraceID:  100,
			expectedParentID: 200,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envTraceID != nil {
				os.Setenv(envTraceID, *tc.envTraceID)
				defer os.Unsetenv(envTraceID)
			}
			if tc.envParentID != nil {
				os.Setenv(envParentID, *tc.envParentID)
				defer os.Unsetenv(envParentID)
			}

			span, ctx := StartSpanFromEnv(context.Background(), "env-operation")
			require.NotNil(t, span, "Expected a span")
			s, ok := SpanFromContext(ctx)
			assert.True(t, ok)
			assert.Equal(t, span, s)

			assert.Equal(t, tc.expectedParentID, span.span.ParentID)
			if tc.expectedTraceID != randTraceID {
				assert.Equal(t, tc.expectedTraceID, span.span.TraceID)
			} else {
				assert.NotEqual(t, 0, span.span.TraceID)
			}

		})
	}
}

func TestLimit(t *testing.T) {
	totalSpans := maxSpansInFlight + 2
	ctx := context.Background()
	for i := 0; i < totalSpans; i++ {
		_, ctx = StartSpanFromContext(ctx, "test")
	}
	assert.Len(t, globalTracer.spans, maxSpansInFlight)
}

func TestEnvFromContext(t *testing.T) {
	s, ctx := StartSpanFromContext(context.Background(), "test")
	s.span.TraceID = 456
	s.span.SpanID = 123
	ctx = setSpanIDsInContext(ctx, s)
	env := EnvFromContext(ctx)
	assert.ElementsMatch(t, []string{"DATADOG_TRACE_ID=456", "DATADOG_PARENT_ID=123"}, env)

	env = EnvFromContext(context.Background())
	assert.ElementsMatch(t, []string{}, env)
}

func TestSpanFinished(t *testing.T) {
	s, _ := StartSpanFromContext(context.Background(), "test")
	s.Finish(nil)
	s.SetResourceName("new")
	s.SetTag("key", "value")

	assert.Equal(t, "test", s.span.Resource)
	_, ok := s.span.Meta["key"]
	assert.False(t, ok)
}

func TestRemapOnFlush(t *testing.T) {
	const testService = "test-service"
	const numTraces = 10
	telem := newTelemetry(&http.Client{}, "api", "datad0g.com", testService)
	globalTracer = &tracer{spans: make(map[uint64]*Span)}

	// traces with 2 spans
	for i := 0; i < numTraces; i++ {
		parentSpan, ctx := StartSpanFromContext(context.Background(), "parent")
		childSpan, _ := StartSpanFromContext(ctx, "child")
		childSpan.Finish(errors.New("test_error"))
		parentSpan.Finish(nil)
	}
	resTraces := telem.extractCompletedSpans()
	require.Len(t, resTraces, numTraces)

	for _, trace := range resTraces {
		assert.Len(t, trace, 2)
		for _, span := range trace {
			assert.Equal(t, testService, span.Service)
			assert.Equal(t, "staging", span.Meta["env"])
			assert.Equal(t, 2.0, span.Metrics["_sampling_priority_v1"])
		}
		var parent, child *span
		if trace[0].Name == "parent" {
			parent = trace[0]
			child = trace[1]
		} else {
			parent = trace[1]
			child = trace[0]
		}
		assert.Equal(t, parent.SpanID, child.ParentID)
		val, ok := parent.Metrics["_top_level"]
		require.True(t, ok)
		require.Equal(t, 1.0, val)
		_, ok = child.Metrics["_top_level"]
		require.False(t, ok)

		require.Equal(t, int32(1), child.Error)
		require.Equal(t, "test_error", child.Meta["error.message"])
		require.Contains(t, child.Meta["error.stack"], "telemetry_test.go")
	}
}

func TestSampling(t *testing.T) {
	const testService = "test-service"
	telem := newTelemetry(&http.Client{}, "api", "datad0g.com", testService)
	globalTracer = &tracer{spans: make(map[uint64]*Span)}

	// Create a span that should be sampled (normal trace ID)
	normalSpan := newSpan("normal", 1234, 1234)
	normalSpan.Finish(nil)

	// Create a span that should be dropped (dropTraceID)
	droppedSpan := newSpan("dropped", 12345, dropTraceID)
	droppedSpan.Finish(nil)

	// Extract completed spans
	resTraces := telem.extractCompletedSpans()
	require.Len(t, resTraces, 1, "Expected only one trace to be completed")

	// Verify the normal span is present
	trace := resTraces[0]
	require.Len(t, trace, 1, "Expected only one span in the trace")
	assert.Equal(t, "normal", trace[0].Name, "Expected the normal span to be present")
	assert.NotEqual(t, dropTraceID, trace[0].TraceID, "Expected the trace ID to not be dropTraceID")
}
