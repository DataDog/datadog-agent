// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"math"
	"strings"
	"unicode"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/logsignature"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// LogMetricsExtractorConfig holds configuration for the LogMetricsExtractor.
type LogMetricsExtractorConfig struct {
	MaxEvalBytes       int     `json:"max_eval_bytes,omitempty"`
	MinTokenMatchRatio float64 `json:"min_token_match_ratio,omitempty"`

	// Cache bounds are internal; non-positive values use the defaults.
	maxPatterns           int
	maxPatternsPerGroup   int
	maxTagGroups          int
	patternIdleTTLSeconds int64

	// IncludeFields, if non-empty, restricts JSON numeric extraction to these field names.
	IncludeFields map[string]struct{} `json:"include_fields,omitempty"`
	// ExcludeFields always excludes JSON fields from numeric extraction.
	ExcludeFields map[string]struct{} `json:"exclude_fields,omitempty"`
}

// DefaultLogMetricsExtractorConfig returns a LogMetricsExtractorConfig with default values.
func DefaultLogMetricsExtractorConfig() LogMetricsExtractorConfig {
	return LogMetricsExtractorConfig{
		MaxEvalBytes:          4096,
		MinTokenMatchRatio:    0.6,
		maxPatterns:           3000,
		maxPatternsPerGroup:   1024,
		maxTagGroups:          256,
		patternIdleTTLSeconds: 4 * 60 * 60,
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
// The engine is the single writer; the cache reuses tokenizer scratch and
// retains only bounded, owned representatives.
type LogMetricsExtractor struct {
	config    LogMetricsExtractorConfig
	patterns  *logsignature.Cache
	telemetry *observerTelemetry
}

// resolveLogMetricsExtractorConfig applies bounded defaults before ingestion.
func resolveLogMetricsExtractorConfig(config LogMetricsExtractorConfig) LogMetricsExtractorConfig {
	defaults := DefaultLogMetricsExtractorConfig()
	if config.MaxEvalBytes <= 0 {
		config.MaxEvalBytes = defaults.MaxEvalBytes
	}
	if config.MinTokenMatchRatio <= 0 || config.MinTokenMatchRatio > 1 || math.IsNaN(config.MinTokenMatchRatio) {
		config.MinTokenMatchRatio = defaults.MinTokenMatchRatio
	}
	if config.maxPatterns <= 0 {
		config.maxPatterns = defaults.maxPatterns
	}
	if config.maxPatternsPerGroup <= 0 {
		config.maxPatternsPerGroup = defaults.maxPatternsPerGroup
	}
	if config.maxTagGroups <= 0 {
		config.maxTagGroups = defaults.maxTagGroups
	}
	if config.patternIdleTTLSeconds <= 0 {
		config.patternIdleTTLSeconds = defaults.patternIdleTTLSeconds
	}
	return config
}

// NewLogMetricsExtractor constructs a bounded pattern cache and JSON field extractor.
func NewLogMetricsExtractor(config LogMetricsExtractorConfig) *LogMetricsExtractor {
	config = resolveLogMetricsExtractorConfig(config)
	return &LogMetricsExtractor{config: config, patterns: logsignature.NewCache(logsignature.CacheConfig{
		MaxEvalBytes: config.MaxEvalBytes, MatchThreshold: config.MinTokenMatchRatio,
		MaxPatterns: config.maxPatterns, MaxPatternsPerGroup: config.maxPatternsPerGroup,
		MaxGroups: config.maxTagGroups, IdleTTLSeconds: config.patternIdleTTLSeconds,
	})}
}

// Reset drops cached representatives when the engine resets its series.
func (a *LogMetricsExtractor) Reset() {
	a.patterns.Reset()
	if a.telemetry != nil {
		a.telemetry.setLogMetricsPatternCount(0)
	}
}

// SetObserverTelemetry publishes the active pattern count without scanning.
func (a *LogMetricsExtractor) SetObserverTelemetry(t *observerTelemetry) {
	a.telemetry = t
	if t != nil {
		t.setLogMetricsPatternCount(a.patterns.Len())
	}
}

func logMetricsGroup(log observer.LogView) logsignature.Group {
	var group logsignature.Group
	for _, tag := range log.Tags() {
		key, value, ok := strings.Cut(tag, ":")
		if !ok {
			continue
		}
		switch key {
		case "source":
			group.Source = value
		case "service":
			group.Service = value
		case "env":
			group.Env = value
		case "host":
			group.Host = value
		}
	}
	if group.Host == "" {
		group.Host = log.GetHostname()
	}
	return group
}

func (a *LogMetricsExtractor) Name() string { return LogMetricsExtractorName }

func (a *LogMetricsExtractor) ProcessLog(log observer.LogView) observer.LogMetricsExtractorOutput {
	content := log.GetContent()
	tags := tagset.CompositeTagsFromSlice(log.Tags())

	pattern, evicted := a.patterns.Match(logMetricsGroup(log), content, log.GetTimestampUnixMilli()/1000)
	if a.telemetry != nil {
		a.telemetry.setLogMetricsPatternCount(a.patterns.Len())
	}
	if pattern == nil {
		return observer.LogMetricsExtractorOutput{EvictedMetricNames: evicted}
	}

	metrics := []observer.MetricOutput{{
		Name:       pattern.SeriesName,
		Value:      1,
		MetricType: pkgmetrics.CountType,
		Tags:       tags,
		HasContext: true,
		Context: observer.MetricContext{
			Pattern: pattern.Signature,
			Example: content,
			Source:  "log_metrics_extractor",
		},
	}}

	// For JSON logs, also extract numeric field metrics
	if isJSONObject(content) {
		metrics = append(metrics, a.extractJSONFieldMetrics(content, tags)...)
	}

	return observer.LogMetricsExtractorOutput{Metrics: metrics, EvictedMetricNames: evicted}
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
