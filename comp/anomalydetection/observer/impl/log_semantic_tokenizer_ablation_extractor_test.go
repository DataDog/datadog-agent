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

func TestLogSemanticTokenizerAblationExtractor_ExactKeepsRawValues(t *testing.T) {
	e := NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MatchMode:                 semanticAblationMatchModeExact,
	})

	first := e.ProcessLog(&mockLogView{content: "request 123 failed", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "request 456 failed", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.NotEqual(t, first.Metrics[0].Name, second.Metrics[0].Name)
}

func TestLogSemanticTokenizerAblationExtractor_AdaptiveMergesByValueRatio(t *testing.T) {
	e := NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MatchThreshold:            0.5,
		MatchMode:                 semanticAblationMatchModeAdaptive,
	})

	first := e.ProcessLog(&mockLogView{content: "request 123 failed", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "request 456 failed", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, first.Metrics[0].Name, second.Metrics[0].Name)
}

func TestLogSemanticTokenizerAblationExtractor_LengthRuleIsIsolated(t *testing.T) {
	adaptive := NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MatchThreshold:            0.9,
		MatchMode:                 semanticAblationMatchModeAdaptive,
	})
	equalLength := NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MatchThreshold:            0.9,
		MatchMode:                 semanticAblationMatchModeEqualLength,
	})

	short := &mockLogView{content: "request failed", tags: []string{"service:api"}}
	long := &mockLogView{content: "request failed now", tags: []string{"service:api"}}
	adaptiveFirst := adaptive.ProcessLog(short)
	adaptiveSecond := adaptive.ProcessLog(long)
	equalFirst := equalLength.ProcessLog(short)
	equalSecond := equalLength.ProcessLog(long)

	assert.Equal(t, adaptiveFirst.Metrics[0].Name, adaptiveSecond.Metrics[0].Name)
	assert.NotEqual(t, equalFirst.Metrics[0].Name, equalSecond.Metrics[0].Name)
}

func TestLogSemanticTokenizerAblationExtractor_ResetClearsPatterns(t *testing.T) {
	e := NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
		MinPatternCountBeforeEmit: 2,
	})
	log := &mockLogView{content: "request 123 failed", tags: []string{"service:api"}}

	require.Empty(t, e.ProcessLog(log).Metrics)
	e.Reset()
	require.Empty(t, e.ProcessLog(log).Metrics)
	require.Len(t, e.ProcessLog(log).Metrics, 1)
}
