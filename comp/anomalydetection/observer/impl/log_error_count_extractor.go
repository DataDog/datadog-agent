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
	if !hasLogErrorCountExtractor(extractors) {
		return nil
	}
	return newMaterializedLogCountBucketizer(LogCountBucketConfig{
		BucketSeconds:    defaultLogCountBucketSeconds,
		RetentionSeconds: defaultLogCountRetentionSeconds,
	})
}

func hasLogErrorCountExtractor(extractors []observerdef.LogMetricsExtractor) bool {
	for _, extractor := range extractors {
		if extractor.Name() == LogErrorCountExtractorName {
			return true
		}
	}
	return false
}

// errorLevelHint cheaply rejects lines that cannot declare an error level, so
// routine output below a source's minimum skips the full parse.
var errorLevelHint = regexp.MustCompile(`(?i)err|crit|fatal|panic|alert|emerg|"(?:` + jsonLevelKeyPattern() + `)"\s*:\s*[56]0|^\s*[EF]\d{4} `)

// jsonLevelKeyPattern matches every key jsonLogLevel reads, so numeric levels
// under any of them reach the full parse.
func jsonLevelKeyPattern() string {
	quoted := make([]string, len(jsonLevelKeys))
	for i, key := range jsonLevelKeys {
		quoted[i] = regexp.QuoteMeta(key)
	}
	return strings.Join(quoted, "|")
}

// declaredErrorStatus raises a below-minimum log to its declared application
// level when that level is ERROR or higher. Other logs keep their transport
// status, so non-error output stays subject to the configured minimum.
func (h *handle) declaredErrorStatus(msg observerdef.LogView, status string, bucket logsfilter.PriorityBucket) (string, logsfilter.PriorityBucket) {
	if h.logFilter == nil || !h.logFilter.admitDeclaredErrors {
		return status, bucket
	}
	content := msg.GetContent()
	if len(content) > 16384 || !errorLevelHint.MatchString(content) {
		return status, bucket
	}
	level, ok := applicationLogSeverity(msg)
	if !ok {
		return status, bucket
	}
	if declared := logsfilter.BucketForStatus(level); declared >= logsfilter.ErrorPriority && declared > bucket {
		return level, declared
	}
	return status, bucket
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

// Header formats are matched only near the start of a line, where loggers put
// the level: klog (`E1116 10:00:00.000000`), logfmt (`level=error`), bracketed
// (`[ERROR]`) and a bare upper-case level after a timestamp (`... ERROR [main]`).
// The earliest match across these and applicationLevelPattern wins, so the
// application's leading level takes precedence over level words in the message.
var (
	klogLevelPattern   = regexp.MustCompile(`^([IWEF])\d{4} \d{2}:\d{2}:\d{2}\.\d+ `)
	headerLevelPattern = regexp.MustCompile(`(?:^|[\s\[(|])(?:(?i:level|lvl|severity|loglevel)=["']?([A-Za-z]+)|\[((?i:TRACE|DEBUG|INFO|NOTICE|WARNING|WARN|ERROR|ERR|CRITICAL|CRIT|FATAL|PANIC))\]|(TRACE|DEBUG|INFO|NOTICE|WARNING|WARN|ERROR|CRITICAL|FATAL|PANIC)(?:\s|$))`)
)

const applicationLevelHeaderBytes = 256

var jsonLevelKeys = []string{"level", "severity", "log.level", "levelname", "lvl", "loglevel"}

var klogLevels = map[string]string{"I": "info", "W": "warn", "E": "error", "F": "fatal"}

// pinoLevels covers the numeric levels used by pino and bunyan.
var pinoLevels = map[int]string{10: "trace", 20: "debug", 30: "info", 40: "warn", 50: "error", 60: "fatal"}

func applicationLogSeverity(log observerdef.LogView) (string, bool) {
	content := strings.TrimSpace(log.GetContent())
	if len(content) > 16384 {
		return "", false
	}
	if strings.HasPrefix(content, "{") {
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(content), &fields) == nil {
			return jsonLogLevel(fields)
		}
	}
	if match := klogLevelPattern.FindStringSubmatch(content); match != nil {
		return klogLevels[match[1]], true
	}
	if level, ok := earliestTextLevel(content); ok {
		return level, true
	}
	// The Agent's own logger supplies application severity directly.
	if log.GetSource() == "datadog-agent" {
		return declaredLogLevel(log.GetStatus())
	}
	return "", false
}

// earliestTextLevel returns the recognized level whose match starts first among
// the header formats (searched near the start of the line) and the `LEVEL:` form.
func earliestTextLevel(content string) (string, bool) {
	start, level := -1, ""
	header := content[:min(len(content), applicationLevelHeaderBytes)]
	if loc := headerLevelPattern.FindStringSubmatchIndex(header); loc != nil {
		for group := 1; 2*group+1 < len(loc); group++ {
			if loc[2*group] < 0 {
				continue
			}
			if declared, ok := declaredLogLevel(header[loc[2*group]:loc[2*group+1]]); ok {
				start, level = loc[0], declared
			}
			break
		}
	}
	if loc := applicationLevelPattern.FindStringSubmatchIndex(content); loc != nil && (start < 0 || loc[0] < start) {
		if declared, ok := declaredLogLevel(content[loc[2]:loc[3]]); ok {
			start, level = loc[0], declared
		}
	}
	return level, start >= 0
}

// jsonLogLevel reads the first recognized level key, ignoring key case.
func jsonLogLevel(fields map[string]json.RawMessage) (string, bool) {
	for _, key := range jsonLevelKeys {
		for name, raw := range fields {
			if !strings.EqualFold(name, key) {
				continue
			}
			var text string
			if json.Unmarshal(raw, &text) == nil {
				if level, ok := declaredLogLevel(text); ok {
					return level, true
				}
			}
			var number int
			if json.Unmarshal(raw, &number) == nil {
				if level, ok := pinoLevels[number]; ok {
					return level, true
				}
			}
		}
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
	case "err":
		return "error", true
	case "crit":
		return "critical", true
	case "emerg":
		return "emergency", true
	case "trace", "debug", "info", "notice", "warn", "warning", "error", "critical", "fatal", "alert", "emergency":
		return value, true
	}
	return "", false
}
