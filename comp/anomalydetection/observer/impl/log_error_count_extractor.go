// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// LogErrorCountExtractorName identifies the aggregate error-count extractor.
const LogErrorCountExtractorName = "log_error_count_extractor"

const logErrorCountMetric = "log.errors.count"

// LogErrorCountExtractor counts accepted error-or-higher logs across patterns.
// Non-error logs contribute zero, establishing history before the first error.
// The engine sums these contributions into completed five-second intervals;
// intervals without accepted logs are unknown and are not synthesized as zero.
type LogErrorCountExtractor struct{}

// Name returns the extractor's storage namespace and catalog identifier.
func (*LogErrorCountExtractor) Name() string { return LogErrorCountExtractorName }

// ProcessLog preserves only service and environment tags. Host and Observer
// ingress source are supplied by the engine. Pattern, container, filename and
// severity tags must not split the aggregate's baseline.
func (*LogErrorCountExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	severity, known := applicationLogSeverity(log)
	if !known {
		return observerdef.LogMetricsExtractorOutput{}
	}
	tags := make([]string, 0, 2)
	for _, tag := range log.Tags() {
		if strings.HasPrefix(tag, "service:") || strings.HasPrefix(tag, "env:") {
			tags = append(tags, tag)
		}
	}
	value := 0.0
	if logsfilter.BucketForStatus(severity) >= logsfilter.ErrorPriority {
		value = 1
	}
	return observerdef.LogMetricsExtractorOutput{Metrics: []observerdef.MetricOutput{{
		Name: logErrorCountMetric, Value: value, Tags: tagset.CompositeTagsFromSlice(tags), MetricType: metrics.CountType,
	}}}
}

// errorCountBucketizer is separate from pattern bucketization: error counts
// must be summed even when optional pattern time buckets are disabled, and
// must never inherit their speculative idle-zero TTL.
func errorCountBucketizer(extractors []observerdef.LogMetricsExtractor) *materializedLogCountBucketizer {
	for _, extractor := range extractors {
		if extractor.Name() == LogErrorCountExtractorName {
			return newMaterializedLogCountBucketizer(LogCountBucketConfig{
				BucketSeconds:    defaultLogCountBucketSeconds,
				RetentionSeconds: defaultLogCountRetentionSeconds,
			})
		}
	}
	return nil
}

// hasErrorCountCapacity bounds pending state even if ingestion does not advance
// data time. Shared storage separately bounds retained series and detector state.
func (b *materializedLogCountBucketizer) hasErrorCountCapacity(seriesKey uint64, timestamp int64) bool {
	const maxPendingSeries = 256
	const maxPendingBuckets = 2
	state := b.series[seriesKey]
	if state == nil {
		return len(b.series) < maxPendingSeries
	}
	end := logCountBucketEnd(timestamp, state.anchor, b.config.BucketSeconds)
	_, exists := state.values[end]
	return exists || len(state.values) < maxPendingBuckets
}

// Explicit application severity takes precedence over transport status. Container
// stderr is not an application error level. Unknown logs establish no zero history.
// Parsing is bounded; production defaults leave this experimental extractor off.
var applicationLevelPattern = regexp.MustCompile(`(?i)(?:^|[\s\]])(TRACE|DEBUG|LOG|INFO|NOTICE|WARNING|WARN|ERROR|CRITICAL|FATAL|PANIC):`)

func applicationLogSeverity(log observerdef.LogView) (string, bool) {
	content := strings.TrimSpace(log.GetContent())
	if len(content) > 16384 {
		return "", false
	}
	if strings.HasPrefix(content, "{") {
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(content), &fields) == nil {
			for _, key := range []string{"level", "severity", "log.level", "levelname"} {
				var value string
				if json.Unmarshal(fields[key], &value) == nil {
					if level, ok := declaredLogLevel(value); ok {
						return level, true
					}
				}
			}
			return "", false
		}
	}
	if match := applicationLevelPattern.FindStringSubmatch(content); len(match) == 2 {
		return declaredLogLevel(match[1])
	}
	// The Agent's own logger supplies application severity directly.
	if log.GetSource() == "datadog-agent" {
		return declaredLogLevel(log.GetStatus())
	}
	return "", false
}

func declaredLogLevel(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "panic":
		return "fatal", true
	case "log":
		return "info", true
	case "trace", "debug", "info", "notice", "warn", "warning", "error", "critical", "fatal", "alert", "emergency":
		return value, true
	}
	return "", false
}
