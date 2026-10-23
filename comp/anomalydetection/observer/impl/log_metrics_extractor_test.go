// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/tokenizer"
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

	// Pattern count is based on the full JSON payload.
	sig := a.logSignature(`{"duration_ms":45,"status":200,"foo":"bar","pid":1234}`)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	expectedCountName := fmt.Sprintf("log.pattern.%x.count", h.Sum64())
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

	// Compute expected metric name (hash of signature).
	sig := a.logSignature("Request completed in 45ms")
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	assert.Equal(t, fmt.Sprintf("log.pattern.%x.count", h.Sum64()), res.Metrics[0].Name)
}

// TestLogMetricsExtractor_LogSignature pins the signatures that name the
// log.pattern.<hash>.count series: if one changes, the series is renamed.
func TestLogMetricsExtractor_LogSignature(t *testing.T) {
	assert.Equal(t, 1, tokenizer.ProfileVersion, "the Logs tokenizer output changed: map new tokens in logSignature back to what AAD printed before, or accept renamed log.pattern series, then update this pin")

	// One extractor for every case, so its tokenizer is reused across logs.
	a := NewLogMetricsExtractor(DefaultLogMetricsExtractorConfig())
	for _, tt := range []struct{ content, want string }{
		{"", ""},
		{"Request completed in 45ms", "CCCCCCC CCCCCCCCC CC DDCC"},
		{"Mon Jan 15 14:30:00 UTC 2024", "DAY MTH DD DD:DD:DD ZONE DDDD"},
		{"ERROR: connection failed", "CCCCC: CCCCCCCCCC CCCCCC"},
		{"WARNING timeout", "CCCCCCC CCCCCCC"},
		{"accepted from 10.0.0.1", "CCCCCCCC CCCC DD.D.D.D"},
		{"accepted from 192.168.100.200:8080", "CCCCCCCC CCCC DDD.DDD.DDD.DDD:DDDD"},
		{"v1.2.3.4.5 error0.0.0.0", "CD.D.D.D.D CCCCCD.D.D.D"},
		{"voilà ok", "CCCCCC CC"},
	} {
		assert.Equal(t, tt.want, a.logSignature(tt.content), "content %q", tt.content)
	}

	capped := NewLogMetricsExtractor(LogMetricsExtractorConfig{MaxEvalBytes: 8})
	assert.Equal(t, "DD.D.D.D", capped.logSignature("10.0.0.1 x"))
	assert.Equal(t, "CCCCCCCC", capped.logSignature("abcdefgh 123"))
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

	sig := a.logSignature(`{"duration_ms":45,"status":200}`)
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
	input := `{"duration_ms":45,`
	log := &mockLogView{content: input, tags: []string{"service:api"}}

	res := a.ProcessLog(log)
	require.Len(t, res.Metrics, 1)

	sig := a.logSignature(input)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	assert.Equal(t, fmt.Sprintf("log.pattern.%x.count", h.Sum64()), res.Metrics[0].Name)
}

func TestLogMetricsExtractor_MetricOutputCarriesInlineContext(t *testing.T) {
	a := &LogMetricsExtractor{}
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
	a := &LogMetricsExtractor{}
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
	require.Equal(t, resA.Metrics[0].Name, resB.Metrics[0].Name)
	require.True(t, resA.Metrics[0].HasContext)
	require.True(t, resB.Metrics[0].HasContext)

	ctxA := resA.Metrics[0].Context
	ctxB := resB.Metrics[0].Context

	assert.Equal(t, "Request completed in 45ms", ctxA.Example)
	assert.Equal(t, "Request completed in 45ms", ctxB.Example)
	assert.Equal(t, ctxA.Pattern, ctxB.Pattern)
}
