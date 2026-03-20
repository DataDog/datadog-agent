// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogMetricsExtractor_JSONNumericExtraction(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{
		ExcludeFields: map[string]struct{}{
			"pid":       {},
			"timestamp": {},
		},
	})

	log := &mockLogView{
		content: []byte(`{"duration_ms":45,"status":200,"foo":"bar","pid":1234}`),
		tags:    []string{"service:api"},
	}

	res := a.ProcessLog(log)
	assert.Len(t, res, 3) // 2 numeric fields + pattern count

	// Order is map iteration dependent; just assert set membership.
	got := map[string]observer.MetricOutput{}
	for _, m := range res {
		got[m.Name] = m
	}

	// Pattern count is based on the full JSON payload.
	sig := logSignature([]byte(`{"duration_ms":45,"status":200,"foo":"bar","pid":1234}`), 0)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	expectedCountName := fmt.Sprintf("log.pattern.%x.count", h.Sum64())
	if m, ok := got[expectedCountName]; assert.True(t, ok) {
		assert.Equal(t, float64(1), m.Value)
		assert.Equal(t, []string{"service:api"}, m.Tags)
	}

	if m, ok := got["log.field.duration_ms"]; assert.True(t, ok) {
		assert.Equal(t, float64(45), m.Value)
		assert.Equal(t, []string{"service:api"}, m.Tags)
	}
	if m, ok := got["log.field.status"]; assert.True(t, ok) {
		assert.Equal(t, float64(200), m.Value)
		assert.Equal(t, []string{"service:api"}, m.Tags)
	}
}

func TestLogMetricsExtractor_UnstructuredPatternCount(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{})

	log := &mockLogView{
		content: []byte("Request completed in 45ms"),
		tags:    []string{"service:web"},
	}

	res := a.ProcessLog(log)
	assert.Len(t, res, 1)
	assert.Equal(t, float64(1), res[0].Value)
	assert.Equal(t, []string{"service:web"}, res[0].Tags)

	// Compute expected metric name (hash of signature).
	sig := logSignature([]byte("Request completed in 45ms"), 0)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	assert.Equal(t, fmt.Sprintf("log.pattern.%x.count", h.Sum64()), res[0].Name)
}

func TestLogMetricsExtractor_JSONIncludeFields(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{
		IncludeFields: map[string]struct{}{
			"duration_ms": {},
		},
	})

	log := &mockLogView{
		content: []byte(`{"duration_ms":45,"status":200}`),
		tags:    []string{"service:api"},
	}

	res := a.ProcessLog(log)
	require.Len(t, res, 2) // selected numeric field + pattern count

	got := map[string]observer.MetricOutput{}
	for _, m := range res {
		got[m.Name] = m
	}

	if m, ok := got["log.field.duration_ms"]; assert.True(t, ok) {
		assert.Equal(t, float64(45), m.Value)
	}

	sig := logSignature([]byte(`{"duration_ms":45,"status":200}`), 0)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	expectedCountName := fmt.Sprintf("log.pattern.%x.count", h.Sum64())
	if m, ok := got[expectedCountName]; assert.True(t, ok) {
		assert.Equal(t, float64(1), m.Value)
	}
}

func TestLogMetricsExtractor_InvalidJSONFallsBackToUnstructured(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{})

	// Looks like JSON but is invalid -> treated as unstructured (pattern frequency).
	input := []byte(`{"duration_ms":45,`)
	log := &mockLogView{content: input, tags: []string{"service:api"}}

	res := a.ProcessLog(log)
	require.Len(t, res, 1)

	sig := logSignature(input, 0)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	assert.Equal(t, fmt.Sprintf("log.pattern.%x.count", h.Sum64()), res[0].Name)
}

func TestLogMetricsExtractor_GetContextByKeyUsesOutputContextKey(t *testing.T) {
	a := &LogMetricsExtractor{}
	log := &mockLogView{
		content: []byte("Request completed in 45ms"),
		tags:    []string{"service:web", "env:prod"},
	}

	res := a.ProcessLog(log)
	require.Len(t, res, 1)
	require.NotEmpty(t, res[0].ContextKey)

	ctx, ok := a.GetContextByKey(res[0].ContextKey)
	require.True(t, ok)
	assert.Equal(t, "log_metrics_extractor", ctx.Source)
	assert.Equal(t, "Request completed in 45ms", ctx.Example)
}

func TestLogMetricsExtractor_ContextKeySeparatesSameMetricByTags(t *testing.T) {
	a := &LogMetricsExtractor{}
	logA := &mockLogView{
		content: []byte("Request completed in 45ms"),
		tags:    []string{"service:api"},
	}
	logB := &mockLogView{
		content: []byte("Request completed in 45ms"),
		tags:    []string{"service:worker"},
	}

	resA := a.ProcessLog(logA)
	resB := a.ProcessLog(logB)
	require.Len(t, resA, 1)
	require.Len(t, resB, 1)
	require.Equal(t, resA[0].Name, resB[0].Name)
	require.NotEqual(t, resA[0].ContextKey, resB[0].ContextKey)

	ctxA, ok := a.GetContextByKey(resA[0].ContextKey)
	require.True(t, ok)
	ctxB, ok := a.GetContextByKey(resB[0].ContextKey)
	require.True(t, ok)

	assert.Equal(t, "Request completed in 45ms", ctxA.Example)
	assert.Equal(t, "Request completed in 45ms", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
}
