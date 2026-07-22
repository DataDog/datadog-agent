// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	semanticpatterns "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/patterns"
	logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

var (
	benchmarkLogsTokens          []logpattern.Token
	benchmarkLogsTokenIndices    []int
	benchmarkSemanticTokens      []semanticpatterns.Token
	benchmarkSemanticMatchResult bool
	benchmarkExtractorOutput     observerdef.LogMetricsExtractorOutput
	benchmarkSemanticCluster     *semanticpatterns.Cluster
)

func tokenizerFactorialCorpus() []*mockLogView {
	logs := make([]*mockLogView, 1000)
	for i := range logs {
		logs[i] = &mockLogView{
			content:     diverseLogContent(i),
			status:      "error",
			tags:        []string{"service:factorial", "env:benchmark", "host:benchmark"},
			hostname:    "benchmark",
			timestampMs: int64(i+1) * 1000,
		}
	}
	return logs
}

// BenchmarkLogTokenizerFactorial separates tokenizer cost, matching cost, and
// the complete extractor paths on the same mixed corpus.
func BenchmarkLogTokenizerFactorial(b *testing.B) {
	logs := tokenizerFactorialCorpus()

	b.Run("tokenize/logs", func(b *testing.B) {
		tokenizer := logpattern.NewTokenizer(defaultLogTokenizerMaxEvalBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkLogsTokens, benchmarkLogsTokenIndices = tokenizer.Tokenize([]byte(logs[i%len(logs)].content))
		}
	})

	b.Run("tokenize/semantic", func(b *testing.B) {
		tokenizer := semanticpatterns.NewTokenizer()
		tokenizer.MaxStringLen = defaultLogTokenizerMaxEvalBytes
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkSemanticTokens = tokenizer.Tokenize(logs[i%len(logs)].content)
		}
	})

	logsTokens := make([][]logpattern.Token, len(logs))
	semanticTokens := make([][]semanticpatterns.Token, len(logs))
	semanticKeys := make([][]semanticValueKey, len(logs))
	logsTokenizer := logpattern.NewTokenizer(defaultLogTokenizerMaxEvalBytes)
	semanticTokenizer := semanticpatterns.NewTokenizer()
	semanticTokenizer.MaxStringLen = defaultLogTokenizerMaxEvalBytes
	for i, log := range logs {
		logsTokens[i], _ = logsTokenizer.Tokenize([]byte(log.content))
		semanticTokens[i] = semanticTokenizer.Tokenize(log.content)
		semanticKeys[i] = semanticValueKeys(semanticTokens[i])
	}

	b.Run("match/logs_positional", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			left := i % len(logsTokens)
			right := (left + 20) % len(logsTokens)
			benchmarkSemanticMatchResult = logpattern.IsMatch(logsTokens[left], logsTokens[right], 0.5)
		}
	})

	b.Run("match/semantic_value_positional", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			left := i % len(semanticKeys)
			right := (left + 20) % len(semanticKeys)
			benchmarkSemanticMatchResult = semanticAblationIsMatch(
				semanticKeys[left], semanticKeys[right], semanticAblationMatchModeAdaptive, 0.5,
			)
		}
	})

	b.Run("cluster/semantic_type_aware_pretokenized", func(b *testing.B) {
		clusterer := semanticpatterns.NewPatternClustererWithTokenizer(semanticTokenizer, 0.5)
		for i := range semanticTokens {
			clusterer.ProcessTokens(semanticTokens[i], logs[i].content, int64(i+1))
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % len(semanticTokens)
			benchmarkSemanticCluster, _ = clusterer.ProcessTokens(semanticTokens[idx], logs[idx].content, int64(i+1))
		}
	})

	fullExtractors := []struct {
		name      string
		extractor observerdef.LogMetricsExtractor
	}{
		{
			name:      "logs_exact",
			extractor: NewLogTokenizerExtractor(DefaultLogTokenizerExtractorConfig()),
		},
		{
			name: "logs_fuzzy_0_5",
			extractor: NewLogTokenizerFuzzyExtractor(LogTokenizerFuzzyExtractorConfig{
				MatchThreshold:            0.5,
				MinPatternCountBeforeEmit: 5,
			}),
		},
		{
			name: "semantic_value_exact",
			extractor: NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
				MatchMode:                 semanticAblationMatchModeExact,
				MinPatternCountBeforeEmit: 5,
			}),
		},
		{
			name: "semantic_value_adaptive_0_5",
			extractor: NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
				MatchThreshold:            0.5,
				MatchMode:                 semanticAblationMatchModeAdaptive,
				MinPatternCountBeforeEmit: 5,
			}),
		},
		{
			name: "semantic_value_equal_length_0_5",
			extractor: NewLogSemanticTokenizerAblationExtractor(LogSemanticTokenizerAblationExtractorConfig{
				MatchThreshold:            0.5,
				MatchMode:                 semanticAblationMatchModeEqualLength,
				MinPatternCountBeforeEmit: 5,
			}),
		},
		{
			name:      "semantic_type_aware_0_5",
			extractor: NewLogPatternExtractor(DefaultLogPatternExtractorConfig()),
		},
	}

	for _, tc := range fullExtractors {
		b.Run("full/"+tc.name, func(b *testing.B) {
			for _, log := range logs {
				tc.extractor.ProcessLog(log)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkExtractorOutput = tc.extractor.ProcessLog(logs[i%len(logs)])
			}
		})
	}
}
