// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strconv"
	"testing"

	logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogTokenizerExtractor_UsesAdaptiveSamplerExactHash(t *testing.T) {
	e := NewLogTokenizerExtractor(LogTokenizerExtractorConfig{MinPatternCountBeforeEmit: 1})
	content := "ERROR request 123 failed"

	out := e.ProcessLog(&mockLogView{content: content, tags: []string{"service:api"}})
	require.Len(t, out.Metrics, 1)

	tokens, _ := logpattern.NewTokenizer(defaultLogTokenizerMaxEvalBytes).Tokenize([]byte(content))
	expectedHash := strconv.FormatUint(logpattern.Hash(tokens), 16)
	assert.Equal(t, "log.log_tokenizer_extractor."+expectedHash+".count", out.Metrics[0].Name)
	assert.Equal(t, logpattern.TokensToString(tokens), out.Metrics[0].Context.Pattern)
	assert.Equal(t, LogTokenizerExtractorName, out.Metrics[0].Context.Source)
}

func TestLogTokenizerExtractor_GroupsEqualExactTokenSequences(t *testing.T) {
	e := NewLogTokenizerExtractor(LogTokenizerExtractorConfig{MinPatternCountBeforeEmit: 1})

	first := e.ProcessLog(&mockLogView{content: "ERROR request 123 failed", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "ERROR another 456 failed", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.Equal(t, first.Metrics[0].Name, second.Metrics[0].Name)
}

func TestLogTokenizerExtractor_SeparatesDifferentExactTokenSequences(t *testing.T) {
	e := NewLogTokenizerExtractor(LogTokenizerExtractorConfig{MinPatternCountBeforeEmit: 1})

	first := e.ProcessLog(&mockLogView{content: "ERROR id 12", tags: []string{"service:api"}})
	second := e.ProcessLog(&mockLogView{content: "ERROR id 123", tags: []string{"service:api"}})

	require.Len(t, first.Metrics, 1)
	require.Len(t, second.Metrics, 1)
	assert.NotEqual(t, first.Metrics[0].Name, second.Metrics[0].Name)
}

func TestLogTokenizerExtractor_DoesNotExtractJSONNumericFields(t *testing.T) {
	e := NewLogTokenizerExtractor(LogTokenizerExtractorConfig{MinPatternCountBeforeEmit: 1})

	out := e.ProcessLog(&mockLogView{
		content: `{"duration_ms":45,"status":500}`,
		tags:    []string{"service:api"},
	})

	require.Len(t, out.Metrics, 1)
	assert.NotContains(t, out.Metrics[0].Name, "log.field.")
}

func TestLogTokenizerExtractor_CountThresholdIsScopedByTagGroup(t *testing.T) {
	e := NewLogTokenizerExtractor(LogTokenizerExtractorConfig{MinPatternCountBeforeEmit: 2})
	content := "ERROR request 123 failed"

	require.Empty(t, e.ProcessLog(&mockLogView{content: content, tags: []string{"service:api"}}).Metrics)
	require.Empty(t, e.ProcessLog(&mockLogView{content: content, tags: []string{"service:worker"}}).Metrics)

	api := e.ProcessLog(&mockLogView{content: content, tags: []string{"service:api"}})
	worker := e.ProcessLog(&mockLogView{content: content, tags: []string{"service:worker"}})
	require.Len(t, api.Metrics, 1)
	require.Len(t, worker.Metrics, 1)
	assert.Equal(t, map[string]string{"service": "api"}, api.Metrics[0].Context.SplitTags)
	assert.Equal(t, map[string]string{"service": "worker"}, worker.Metrics[0].Context.SplitTags)
}

func TestLogTokenizerExtractor_ResetClearsCounts(t *testing.T) {
	e := NewLogTokenizerExtractor(LogTokenizerExtractorConfig{MinPatternCountBeforeEmit: 2})
	log := &mockLogView{content: "ERROR request 123 failed", tags: []string{"service:api"}}

	require.Empty(t, e.ProcessLog(log).Metrics)
	e.Reset()
	require.Empty(t, e.ProcessLog(log).Metrics)
	require.Len(t, e.ProcessLog(log).Metrics, 1)
}
