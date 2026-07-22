// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogTokenizerFuzzyExtractor_MergesAtAdaptiveSamplerThreshold(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 1})

	first := e.ProcessLog(&mockLogView{content: "aa bb cc dd ee", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "aaa bb cc dd ee", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, first.Metrics[0].Name, second.Metrics[0].Name)
	assert.Equal(t, first.Metrics[0].Context.Pattern, second.Metrics[0].Context.Pattern)
}

func TestLogTokenizerFuzzyExtractor_SeparatesBelowThreshold(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 1})

	first := e.ProcessLog(&mockLogView{content: "aa bb cc dd ee", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "aaa bbb cc dd ee", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.NotEqual(t, first.Metrics[0].Name, second.Metrics[0].Name)
}

func TestLogTokenizerFuzzyExtractor_ThresholdIsScopedByTagGroup(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 2})
	content := "aa bb cc dd ee"

	require.Empty(t, e.ProcessLog(&mockLogView{content: content, tags: []string{"service:api"}}).Metrics)
	require.Empty(t, e.ProcessLog(&mockLogView{content: content, tags: []string{"service:worker"}}).Metrics)
	require.Len(t, e.ProcessLog(&mockLogView{content: "aaa bb cc dd ee", tags: []string{"service:api"}}).Metrics, 1)
	require.Len(t, e.ProcessLog(&mockLogView{content: "aaa bb cc dd ee", tags: []string{"service:worker"}}).Metrics, 1)
}

func TestLogTokenizerFuzzyExtractor_ResetClearsPatterns(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 2})
	log := &mockLogView{content: "aa bb cc dd ee", tags: []string{"service:api"}}

	require.Empty(t, e.ProcessLog(log).Metrics)
	e.Reset()
	require.Empty(t, e.ProcessLog(log).Metrics)
	require.Len(t, e.ProcessLog(log).Metrics, 1)
}
