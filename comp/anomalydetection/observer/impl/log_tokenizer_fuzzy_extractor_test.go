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

func TestLogTokenizerFuzzyExtractorDefaultsToHalfTokenMatch(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{})

	assert.Equal(t, 0.5, e.config.MatchThreshold)
}

func TestLogTokenizerFuzzyExtractorMergesAtDefaultThreshold(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 1})

	first := e.ProcessLog(&mockLogView{content: "aa bb cc dd ee", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "aaa bb cc dd ee", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, first.Metrics[0].Name, second.Metrics[0].Name)
	assert.Equal(t, first.Metrics[0].Context.Pattern, second.Metrics[0].Context.Pattern)
}

func TestLogTokenizerFuzzyExtractorSeparatesBelowThreshold(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 1})

	first := e.ProcessLog(&mockLogView{content: "aa bb cc dd ee", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "aaa bbb cccc ddddd eeeeee", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.NotEqual(t, first.Metrics[0].Name, second.Metrics[0].Name)
}

func TestLogTokenizerFuzzyExtractorUUIDsMatchExactly(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MatchThreshold:            1,
	})

	first := e.ProcessLog(&mockLogView{
		content: "Received event event_id=c05d056c-1c1f-457f-bfd2-f381f7f84e0d",
		tags:    []string{"service:cassandra"},
	})
	second := e.ProcessLog(&mockLogView{
		content: "Received event event_id=8b08ddbc-9833-44c8-af9d-eb540fc69041",
		tags:    []string{"service:cassandra"},
	})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, first.Metrics[0].Name, second.Metrics[0].Name)
	assert.Contains(t, first.Metrics[0].Context.Pattern, "UUID")
}

func TestLogTokenizerFuzzyExtractorScopesPatternsByTagGroup(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 2})
	content := "aa bb cc dd ee"

	require.Empty(t, e.ProcessLog(&mockLogView{content: content, tags: []string{"service:api"}}).Metrics)
	require.Empty(t, e.ProcessLog(&mockLogView{content: content, tags: []string{"service:worker"}}).Metrics)
	api := e.ProcessLog(&mockLogView{content: "aaa bb cc dd ee", tags: []string{"service:api"}})
	worker := e.ProcessLog(&mockLogView{content: "aaa bb cc dd ee", tags: []string{"service:worker"}})

	require.Len(t, api.Metrics, 1)
	require.Len(t, worker.Metrics, 1)
	assert.NotEqual(t, api.Metrics[0].Name, worker.Metrics[0].Name)
	assert.Equal(t, map[string]string{"service": "api"}, api.Metrics[0].Context.SplitTags)
}

func TestLogTokenizerFuzzyExtractorResetClearsPatterns(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{MinPatternCountBeforeEmit: 2})
	log := &mockLogView{content: "aa bb cc dd ee", tags: []string{"service:api"}}

	require.Empty(t, e.ProcessLog(log).Metrics)
	e.Reset()
	require.Empty(t, e.ProcessLog(log).Metrics)
	require.Len(t, e.ProcessLog(log).Metrics, 1)
}

func TestLogTokenizerFuzzyExtractorEvictsOldestPatternAtGroupCap(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MatchThreshold:            1,
		MaxPatternsPerGroup:       1,
	})

	first := e.ProcessLog(&mockLogView{content: "letters", tags: []string{"service:api"}, timestampMs: 1000})
	second := e.ProcessLog(&mockLogView{content: "12345", tags: []string{"service:api"}, timestampMs: 2000})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, []string{first.Metrics[0].Name}, second.EvictedMetricNames)
	require.Len(t, e.groups, 1)
	for _, group := range e.groups {
		assert.Len(t, group.patterns, 1)
	}
}

func TestLogTokenizerFuzzyExtractorEvictsTagGroupAtCap(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{
		MinPatternCountBeforeEmit: 1,
		MaxTagGroups:              1,
	})

	first := e.ProcessLog(&mockLogView{content: "request failed", tags: []string{"service:api"}, timestampMs: 1000})
	second := e.ProcessLog(&mockLogView{content: "request failed", tags: []string{"service:worker"}, timestampMs: 2000})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, []string{first.Metrics[0].Name}, second.EvictedMetricNames)
	assert.Len(t, e.groups, 1)
}

func TestLogTokenizerFuzzyExtractorGarbageCollectsStalePatterns(t *testing.T) {
	e := NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{
		MinPatternCountBeforeEmit:    1,
		MatchThreshold:               1,
		PatternTimeToLiveSec:         10,
		GarbageCollectionIntervalSec: 1,
	})

	first := e.ProcessLog(&mockLogView{content: "letters", tags: []string{"service:api"}, timestampMs: 100_000})
	second := e.ProcessLog(&mockLogView{content: "12345", tags: []string{"service:api"}, timestampMs: 111_000})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, []string{first.Metrics[0].Name}, second.EvictedMetricNames)
}
