// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

// LogTimeSeriesAnalysis converts logs into timeseries metric outputs:
// - JSON logs: numeric fields -> Avg aggregation
// - Unstructured logs: pattern frequency -> Sum aggregation
//
// This is intentionally minimal; cardinality controls live in the observer storage (Step 5).
type LogTimeSeriesAnalysis struct {
	// MaxEvalBytes caps how many bytes we evaluate for unstructured signature generation (0 = no cap).
	MaxEvalBytes int

	// IncludeFields, if non-empty, restricts JSON numeric extraction to these field names.
	IncludeFields map[string]struct{}
	// ExcludeFields always excludes JSON fields from numeric extraction.
	ExcludeFields map[string]struct{}
}

func (a *LogTimeSeriesAnalysis) Name() string { return "log_timeseries" }

func (a *LogTimeSeriesAnalysis) Analyze(log observer.LogView) observer.LogAnalysisResult {
	content := log.GetContent()
	tags := log.GetTags()

	// Structured (JSON) path
	if isJSONObject(content) {
		metrics := a.extractJSONNumericFields(content, tags)
		return observer.LogAnalysisResult{Metrics: metrics}
	}

	// Unstructured path: pattern frequency
	patternSig := pattern.Signature(content, a.MaxEvalBytes)
	if patternSig == "" {
		return observer.LogAnalysisResult{}
	}

	return observer.LogAnalysisResult{
		Metrics: []observer.MetricOutput{{
			Name:        patternCountMetricName(patternSig),
			Value:       1,
			Tags:        tags,
			Aggregation: observer.AggregationSum,
		}},
	}
}

func isJSONObject(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	return len(trimmed) > 1 && trimmed[0] == '{' && json.Valid(trimmed)
}

func (a *LogTimeSeriesAnalysis) extractJSONNumericFields(content []byte, tags []string) []observer.MetricOutput {
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil
	}

	var out []observer.MetricOutput
	for k, v := range obj {
		if a.ExcludeFields != nil {
			if _, ok := a.ExcludeFields[k]; ok {
				continue
			}
		}
		if len(a.IncludeFields) > 0 {
			if _, ok := a.IncludeFields[k]; !ok {
				continue
			}
		}

		f, ok := coerceNumber(v)
		if !ok {
			continue
		}

		out = append(out, observer.MetricOutput{
			Name:        "log.field." + sanitizeMetricFragment(k),
			Value:       f,
			Tags:        tags,
			Aggregation: observer.AggregationAvg,
		})
	}

	return out
}

func coerceNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		// Prefer float to support decimals; json.Number will parse ints as well.
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint:
		return float64(n), true
	default:
		return 0, false
	}
}

func patternCountMetricName(signature string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(signature))
	return fmt.Sprintf("log.pattern.%x.count", h.Sum64())
}

func sanitizeMetricFragment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
