// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogTimeSeriesAnalysis_JSONNumericExtraction(t *testing.T) {
	a := &LogTimeSeriesAnalysis{
		ExcludeFields: map[string]struct{}{
			"pid":       {},
			"timestamp": {},
		},
	}

	log := &mockLogView{
		content: []byte(`{"duration_ms":45,"status":200,"foo":"bar","pid":1234}`),
		tags:    []string{"service:api"},
	}

	res := a.Process(log)
	assert.Len(t, res.Metrics, 3) // 2 numeric fields + pattern count

	// Order is map iteration dependent; just assert set membership.
	got := map[string]observer.MetricOutput{}
	for _, m := range res.Metrics {
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

func TestLogTimeSeriesAnalysis_UnstructuredPatternCount(t *testing.T) {
	a := &LogTimeSeriesAnalysis{MaxEvalBytes: 0}

	log := &mockLogView{
		content: []byte("Request completed in 45ms"),
		tags:    []string{"service:web"},
	}

	res := a.Process(log)
	assert.Len(t, res.Metrics, 1)
	assert.Equal(t, float64(1), res.Metrics[0].Value)
	assert.Equal(t, []string{"service:web"}, res.Metrics[0].Tags)

	// Compute expected metric name (hash of signature).
	sig := logSignature([]byte("Request completed in 45ms"), 0)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	assert.Equal(t, fmt.Sprintf("log.pattern.%x.count", h.Sum64()), res.Metrics[0].Name)
}

func TestLogTimeSeriesAnalysis_JSONIncludeFields(t *testing.T) {
	a := &LogTimeSeriesAnalysis{
		IncludeFields: map[string]struct{}{
			"duration_ms": {},
		},
	}

	log := &mockLogView{
		content: []byte(`{"duration_ms":45,"status":200}`),
		tags:    []string{"service:api"},
	}

	res := a.Process(log)
	require.Len(t, res.Metrics, 2) // selected numeric field + pattern count

	got := map[string]observer.MetricOutput{}
	for _, m := range res.Metrics {
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

func TestLogTimeSeriesAnalysis_InvalidJSONFallsBackToUnstructured(t *testing.T) {
	a := &LogTimeSeriesAnalysis{MaxEvalBytes: 0}

	// Looks like JSON but is invalid -> treated as unstructured (pattern frequency).
	input := []byte(`{"duration_ms":45,`)
	log := &mockLogView{content: input, tags: []string{"service:api"}}

	res := a.Process(log)
	require.Len(t, res.Metrics, 1)

	sig := logSignature(input, 0)
	h := fnv.New64a()
	_, _ = h.Write([]byte(sig))
	assert.Equal(t, fmt.Sprintf("log.pattern.%x.count", h.Sum64()), res.Metrics[0].Name)
}
