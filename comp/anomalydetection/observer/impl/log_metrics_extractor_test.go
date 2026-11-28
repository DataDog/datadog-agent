// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestLogMetricsExtractor_JSONNumericExtraction(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{
		ExcludeFields: map[string]struct{}{
			"pid":       {},
			"timestamp": {},
		},
	})

	log := &mockLogView{
		content: `{"duration_ms":45,"status":200,"foo":"bar","pid":1234}`,
		tags:    []string{"service:api"},
	}

	res := a.ProcessLog(log)
	assert.Len(t, res.Metrics, 3) // 2 numeric fields + pattern count

	// Order is map iteration dependent; just assert set membership.
	got := map[string]observer.MetricOutput{}
	for _, m := range res.Metrics {
		got[m.Name] = m
	}

	expectedCountName := res.Metrics[0].Name
	assert.True(t, strings.HasPrefix(expectedCountName, "log.pattern."))
	if m, ok := got[expectedCountName]; assert.True(t, ok) {
		assert.Equal(t, float64(1), m.Value)
		assert.Equal(t, tagset.CompositeTagsFromSlice([]string{"service:api"}), m.Tags)
	}

	if m, ok := got["log.field.duration_ms"]; assert.True(t, ok) {
		assert.Equal(t, float64(45), m.Value)
		assert.Equal(t, tagset.CompositeTagsFromSlice([]string{"service:api"}), m.Tags)
	}
	if m, ok := got["log.field.status"]; assert.True(t, ok) {
		assert.Equal(t, float64(200), m.Value)
		assert.Equal(t, tagset.CompositeTagsFromSlice([]string{"service:api"}), m.Tags)
	}
}

func TestLogMetricsExtractor_UnstructuredPatternCount(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{})

	log := &mockLogView{
		content: "Request completed in 45ms",
		tags:    []string{"service:web"},
	}

	res := a.ProcessLog(log)
	assert.Len(t, res.Metrics, 1)
	assert.Equal(t, float64(1), res.Metrics[0].Value)
	assert.Equal(t, tagset.CompositeTagsFromSlice([]string{"service:web"}), res.Metrics[0].Tags)

	assert.True(t, strings.HasPrefix(res.Metrics[0].Name, "log.pattern."))
	assert.True(t, strings.HasSuffix(res.Metrics[0].Name, ".count"))
}

func TestLogMetricsExtractor_JSONIncludeFields(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{
		IncludeFields: map[string]struct{}{
			"duration_ms": {},
		},
	})

	log := &mockLogView{
		content: `{"duration_ms":45,"status":200}`,
		tags:    []string{"service:api"},
	}

	res := a.ProcessLog(log)
	require.Len(t, res.Metrics, 2) // selected numeric field + pattern count

	got := map[string]observer.MetricOutput{}
	for _, m := range res.Metrics {
		got[m.Name] = m
	}

	if m, ok := got["log.field.duration_ms"]; assert.True(t, ok) {
		assert.Equal(t, float64(45), m.Value)
	}

	expectedCountName := res.Metrics[0].Name
	if m, ok := got[expectedCountName]; assert.True(t, ok) {
		assert.Equal(t, float64(1), m.Value)
	}
}

func TestLogMetricsExtractor_InvalidJSONFallsBackToUnstructured(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{})

	// Looks like JSON but is invalid -> treated as unstructured (pattern frequency).
	input := `{"duration_ms":45,`
	log := &mockLogView{content: input, tags: []string{"service:api"}}

	res := a.ProcessLog(log)
	require.Len(t, res.Metrics, 1)

	assert.True(t, strings.HasPrefix(res.Metrics[0].Name, "log.pattern."))
}

func TestLogMetricsExtractor_MetricOutputCarriesInlineContext(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{})
	log := &mockLogView{
		content: "Request completed in 45ms",
		tags:    []string{"service:web", "env:prod"},
	}

	res := a.ProcessLog(log)
	require.Len(t, res.Metrics, 1)
	require.True(t, res.Metrics[0].HasContext)

	ctx := res.Metrics[0].Context
	assert.Equal(t, "log_metrics_extractor", ctx.Source)
	assert.Equal(t, "Request completed in 45ms", ctx.Example)
}

func TestLogMetricsExtractor_ContextDiffersPerTagSet(t *testing.T) {
	a := NewLogMetricsExtractor(LogMetricsExtractorConfig{})
	logA := &mockLogView{
		content: "Request completed in 45ms",
		tags:    []string{"service:api"},
	}
	logB := &mockLogView{
		content: "Request completed in 45ms",
		tags:    []string{"service:worker"},
	}

	resA := a.ProcessLog(logA)
	resB := a.ProcessLog(logB)
	require.Len(t, resA.Metrics, 1)
	require.Len(t, resB.Metrics, 1)
	require.NotEqual(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.True(t, resA.Metrics[0].HasContext)
	require.True(t, resB.Metrics[0].HasContext)

	ctxA := resA.Metrics[0].Context
	ctxB := resB.Metrics[0].Context

	assert.Equal(t, "Request completed in 45ms", ctxA.Example)
	assert.Equal(t, "Request completed in 45ms", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
}

func TestLogMetricsExtractor_MaxEvalBytesCapsSignature(t *testing.T) {
	// Logs that differ only after the first MaxEvalBytes bytes share a series.
	prefix := strings.Repeat("a", 16)
	capped := NewLogMetricsExtractor(LogMetricsExtractorConfig{MaxEvalBytes: len(prefix), MinTokenMatchRatio: 1})
	a := capped.ProcessLog(&mockLogView{content: prefix + " 123"})
	b := capped.ProcessLog(&mockLogView{content: prefix + " xyz"})
	require.Len(t, a.Metrics, 1)
	require.Len(t, b.Metrics, 1)
	assert.Equal(t, a.Metrics[0].Name, b.Metrics[0].Name)

	uncapped := NewLogMetricsExtractor(LogMetricsExtractorConfig{MinTokenMatchRatio: 1})
	c := uncapped.ProcessLog(&mockLogView{content: prefix + " 123"})
	d := uncapped.ProcessLog(&mockLogView{content: prefix + " xyz"})
	assert.NotEqual(t, c.Metrics[0].Name, d.Metrics[0].Name)
}

func TestLogMetricsExtractor_PropagatesServiceWithoutTagMutation(t *testing.T) {
	extractor := NewLogMetricsExtractor(LogMetricsExtractorConfig{})

	jsonRes := extractor.ProcessLog(&mockLogView{
		service: "checkout",
		content: `{"latency_ms":42}`,
	})
	require.Len(t, jsonRes.Metrics, 2)
	for _, metric := range jsonRes.Metrics {
		require.True(t, metric.HasContext)
		assert.Equal(t, "checkout", metric.Context.Service)
		assert.Empty(t, metric.Tags.Len())
	}

	otherRes := extractor.ProcessLog(&mockLogView{
		service: "payments",
		content: `{"latency_ms":42}`,
	})
	require.Len(t, otherRes.Metrics, 2)
	assert.Equal(t, "payments", otherRes.Metrics[0].Context.Service)
	assert.Equal(t, "payments", otherRes.Metrics[1].Context.Service)
	assert.NotEqual(t, jsonRes.Metrics[0].Name, otherRes.Metrics[0].Name)

	originWins := extractor.ProcessLog(&mockLogView{
		service: "origin-svc",
		tags:    []string{"service:tag-svc"},
		content: `{"latency_ms":42}`,
	})
	require.Len(t, originWins.Metrics, 2)
	for _, metric := range originWins.Metrics {
		assert.Equal(t, "origin-svc", metric.Context.Service)
	}

	tagFallback := extractor.ProcessLog(&mockLogView{
		tags:    []string{"service:tag-svc"},
		content: `{"latency_ms":42}`,
	})
	require.Len(t, tagFallback.Metrics, 2)
	for _, metric := range tagFallback.Metrics {
		assert.Equal(t, "tag-svc", metric.Context.Service)
	}
}
