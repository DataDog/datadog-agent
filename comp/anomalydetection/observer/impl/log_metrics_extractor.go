// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/logsignature"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// LogMetricsExtractorConfig holds configuration for the LogMetricsExtractor.
type LogMetricsExtractorConfig struct {
	// MaxEvalBytes caps how many bytes we evaluate for unstructured signature generation (0 = no cap).
	MaxEvalBytes int

	// IncludeFields, if non-empty, restricts JSON numeric extraction to these field names.
	IncludeFields map[string]struct{}
	// ExcludeFields always excludes JSON fields from numeric extraction.
	ExcludeFields map[string]struct{}
}

// DefaultLogMetricsExtractorConfig returns a LogMetricsExtractorConfig with default values.
func DefaultLogMetricsExtractorConfig() LogMetricsExtractorConfig {
	return LogMetricsExtractorConfig{
		MaxEvalBytes: 4096,
		ExcludeFields: map[string]struct{}{
			"timestamp": {}, "ts": {}, "time": {},
			"pid": {}, "ppid": {}, "uid": {}, "gid": {},
		},
	}
}

// LogMetricsExtractorName is the canonical name for the log metrics extractor.
const LogMetricsExtractorName = "log_metrics_extractor"

// LogMetricsExtractor converts logs into timeseries metric outputs:
// - JSON logs: numeric fields -> Avg aggregation
// - Unstructured logs: pattern frequency -> Sum aggregation
//
// This is intentionally minimal; cardinality controls live in the observer storage (Step 5).
type LogMetricsExtractor struct {
	config             LogMetricsExtractorConfig
	signatureGenerator *logsignature.Generator
}

// NewLogMetricsExtractor creates a LogMetricsExtractor with the given config.
func NewLogMetricsExtractor(config LogMetricsExtractorConfig) *LogMetricsExtractor {
	return &LogMetricsExtractor{config: config, signatureGenerator: logsignature.NewGenerator(config.MaxEvalBytes)}
}

func (a *LogMetricsExtractor) Name() string { return LogMetricsExtractorName }

func (a *LogMetricsExtractor) ProcessLog(log observer.LogView) observer.LogMetricsExtractorOutput {
	content := log.GetContent()
	tags := tagset.CompositeTagsFromSlice(log.Tags())

	// Always emit pattern frequency metric for all logs
	patternSig := a.signatureGenerator.Generate(content)
	if patternSig == "" {
		return observer.LogMetricsExtractorOutput{}
	}

	metricName := patternCountMetricName(patternSig)

	metrics := []observer.MetricOutput{{
		Name:       metricName,
		Value:      1,
		MetricType: pkgmetrics.CountType,
		Tags:       tags,
		HasContext: true,
		Context: observer.MetricContext{
			Pattern: patternSig,
			Example: content,
			Source:  "log_metrics_extractor",
		},
	}}

	// For JSON logs, also extract numeric field metrics
	if isJSONObject(content) {
		metrics = append(metrics, a.extractJSONFieldMetrics(content, tags)...)
	}

	return observer.LogMetricsExtractorOutput{Metrics: metrics}
}

func isJSONObject(s string) bool {
	trimmed := strings.TrimSpace(s)
	return len(trimmed) > 1 && trimmed[0] == '{' && json.Valid([]byte(trimmed))
}

// extractJSONFieldMetrics extracts numeric field metrics from JSON content.
func (a *LogMetricsExtractor) extractJSONFieldMetrics(content string, tags tagset.CompositeTags) []observer.MetricOutput {
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil
	}

	example := truncate(content, 160)
	var out []observer.MetricOutput
	for k, v := range obj {
		if a.config.ExcludeFields != nil {
			if _, ok := a.config.ExcludeFields[k]; ok {
				continue
			}
		}
		if len(a.config.IncludeFields) > 0 {
			if _, ok := a.config.IncludeFields[k]; !ok {
				continue
			}
		}

		f, ok := coerceNumber(v)
		if !ok {
			continue
		}

		out = append(out, observer.MetricOutput{
			Name:       "log.field." + sanitizeMetricFragment(k),
			Value:      f,
			MetricType: pkgmetrics.GaugeType,
			Tags:       tags,
			HasContext: true,
			Context: observer.MetricContext{
				Pattern: k,
				Example: example,
				Source:  "log_metrics_extractor",
			},
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
	return "log.pattern." + strconv.FormatUint(fnv64aString(signature), 16) + ".count"
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
