// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"regexp"
	"strconv"
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
// Only errors contribute: log_failure counts error intervals and has no use for
// zero history, so a scope that never errors creates no series or pending state.
// The engine sums contributions into completed five-second intervals.
type LogErrorCountExtractor struct{}

// Name returns the extractor's storage namespace and catalog identifier.
func (*LogErrorCountExtractor) Name() string { return LogErrorCountExtractorName }

// ProcessLog preserves only service and environment tags. Host and Observer
// ingress source are supplied by the engine. Pattern, container, filename and
// severity tags must not split the aggregate's baseline.
func (*LogErrorCountExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	severity, known := applicationLogSeverity(log)
	if !known || logsfilter.BucketForStatus(severity) < logsfilter.ErrorPriority {
		return observerdef.LogMetricsExtractorOutput{}
	}
	tags := make([]string, 0, 2)
	for _, tag := range log.Tags() {
		if strings.HasPrefix(tag, "service:") || strings.HasPrefix(tag, "env:") {
			tags = append(tags, tag)
		}
	}
	return observerdef.LogMetricsExtractorOutput{Metrics: []observerdef.MetricOutput{{
		Name: logErrorCountMetric, Value: 1, Tags: tagset.CompositeTagsFromSlice(tags), MetricType: metrics.CountType,
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

// mayDeclareErrorLevel cheaply rejects lines that cannot declare an error
// level, so routine output below a source's minimum skips the full parse. It
// accepts err, crit, fatal, panic, alert or emerg anywhere, ignoring case; a
// quoted level key with the numeric value 50 or 60; or a leading klog E/F
// header. It is a plain scan because a regexp cannot skip ahead for these
// alternatives and would examine every byte of every routine line.
func mayDeclareErrorLevel(s string) bool {
	if t := strings.TrimLeft(s, levelSpace); len(t) > 5 && (t[0]|0x20 == 'e' || t[0]|0x20 == 'f') && isDigits(t[1:5]) && t[5] == ' ' {
		return true
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '"' && numericErrorLevelAt(s[i+1:]) {
			return true
		}
		if c := s[i] | 0x20; c == 'e' || c == 'c' || c == 'f' || c == 'p' || c == 'a' {
			for _, word := range errorLevelWords {
				if word[0] == c && hasPrefixFold(s[i:], word) {
					return true
				}
			}
		}
	}
	return false
}

var errorLevelWords = []string{"err", "emerg", "crit", "fatal", "panic", "alert"}

// numericErrorLevelAt reports whether s, which follows an opening quote, starts
// with a key jsonLogLevel reads and a numeric pino/bunyan error or fatal level.
func numericErrorLevelAt(s string) bool {
	for _, key := range jsonLevelKeys {
		if !hasPrefixFold(s, key) || len(s) == len(key) || s[len(key)] != '"' {
			continue
		}
		rest := strings.TrimLeft(s[len(key)+1:], levelSpace)
		if rest, ok := strings.CutPrefix(rest, ":"); ok {
			rest = strings.TrimLeft(rest, levelSpace)
			if len(rest) >= 2 && (rest[0] == '5' || rest[0] == '6') && rest[1] == '0' {
				return true
			}
		}
	}
	return false
}

// levelSpace is the whitespace class used by level parsing (RE2's \s).
const levelSpace = " \t\n\f\r"

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// declaredErrorStatus raises a below-minimum log to its declared application
// level when that level is ERROR or higher. Other logs keep their transport
// status, so non-error output stays subject to the configured minimum.
func (h *handle) declaredErrorStatus(msg observerdef.LogView, status string, bucket logsfilter.PriorityBucket) (string, logsfilter.PriorityBucket) {
	if h.logFilter == nil || !h.logFilter.admitDeclaredErrors {
		return status, bucket
	}
	content := msg.GetContent()
	if len(content) > 16384 || !mayDeclareErrorLevel(content) {
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
// stderr is not an application error level.
// Parsing is bounded; production defaults leave this experimental extractor off.
//
// Text levels are read only from the line header, where loggers put them: klog
// (`E1116 10:00:00.000000`), logfmt (`level=error`), bracketed (`[ERROR]`), and a
// level word in the leading position (see leadingLevel). The earliest match wins,
// so the application's own level takes precedence over level words in the message.
var (
	klogLevelPattern   = regexp.MustCompile(`^([IWEF])\d{4} \d{2}:\d{2}:\d{2}\.\d+ `)
	headerLevelPattern = regexp.MustCompile(`(?:^|[\s\[(|])(?:(?i:level|lvl|severity|loglevel)=["']?([A-Za-z]+)|\[((?i:TRACE|DEBUG|INFO|NOTICE|WARNING|WARN|ERROR|ERR|CRITICAL|CRIT|FATAL|PANIC))\])`)
)

// levelWords count as a bare upper-case word, or in any case before a colon.
// LOG, PostgreSQL's informational level, counts only before a colon.
var levelWords = []string{"TRACE", "DEBUG", "INFO", "NOTICE", "WARNING", "WARN", "ERROR", "CRITICAL", "FATAL", "PANIC", "LOG"}

const applicationLevelHeaderBytes = 256

// jsonLevelKeys are read in priority order, ignoring key case.
var jsonLevelKeys = [...]string{"level", "severity", "log.level", "levelname", "lvl", "loglevel"}

var klogLevels = map[string]string{"I": "info", "W": "warn", "E": "error", "F": "fatal"}

// pinoLevels covers the numeric levels used by pino and bunyan.
var pinoLevels = map[int]string{10: "trace", 20: "debug", 30: "info", 40: "warn", 50: "error", 60: "fatal"}

func applicationLogSeverity(log observerdef.LogView) (string, bool) {
	content := strings.TrimSpace(log.GetContent())
	if len(content) > 16384 {
		return "", false
	}
	if strings.HasPrefix(content, "{") && json.Valid([]byte(content)) {
		return jsonLogLevel(content)
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
// the marked header formats and the leading level word.
func earliestTextLevel(content string) (string, bool) {
	header := content[:min(len(content), applicationLevelHeaderBytes)]
	level, start, ok := leadingLevel(header)
	if loc := headerLevelPattern.FindStringSubmatchIndex(header); loc != nil && (!ok || loc[0] < start) {
		for group := 1; 2*group+1 < len(loc); group++ {
			if loc[2*group] < 0 {
				continue
			}
			if declared, valid := declaredLogLevel(header[loc[2*group]:loc[2*group+1]]); valid {
				return declared, true
			}
			break
		}
	}
	return level, ok
}

// leadingLevel reads a level word from the leading position of a line: after
// any timestamps, numbers, time zones, separators and bracketed fields, and at
// most one other word such as a logger name. A level word later in the line is
// message text, as in `previous response was ERROR: timeout`.
func leadingLevel(header string) (level string, start int, ok bool) {
	otherWords := 0
	for i := 0; i < len(header); {
		for i < len(header) && strings.IndexByte(levelSpace, header[i]) >= 0 {
			i++
		}
		start = i
		if i < len(header) && (header[i] == '[' || header[i] == '(') {
			closing := byte(']')
			if header[i] == '(' {
				closing = ')'
			}
			end := strings.IndexByte(header[i+1:], closing)
			if end < 0 {
				break
			}
			i += end + 2
			continue
		}
		for i < len(header) && strings.IndexByte(levelSpace, header[i]) < 0 {
			i++
		}
		word := header[start:i]
		if word == "" {
			break
		}
		if level, ok := levelWord(word); ok {
			return level, start, true
		}
		if word[0] >= '0' && word[0] <= '9' || word == "UTC" || word == "GMT" || strings.Trim(word, "-|:>") == "" {
			continue
		}
		if otherWords++; otherWords > 1 {
			break
		}
	}
	return "", -1, false
}

// levelWord accepts `ERROR`, `ERROR:` and `error:`, including `ERROR:root:...`.
func levelWord(word string) (string, bool) {
	name, _, colon := strings.Cut(word, ":")
	for _, candidate := range levelWords {
		if colon && strings.EqualFold(name, candidate) || !colon && candidate != "LOG" && word == candidate {
			return declaredLogLevel(candidate)
		}
	}
	return "", false
}

// jsonLogLevel reads the first recognized top-level level key from a valid JSON
// object without decoding the object into a map. A repeated key's last value
// wins, as in map decoding.
func jsonLogLevel(object string) (string, bool) {
	var values [len(jsonLevelKeys)]string
	for i := skipJSONSpace(object, 1); object[i] != '}'; {
		keyEnd := jsonValueEnd(object, i)
		key := object[i+1 : keyEnd-1]
		if strings.IndexByte(key, '\\') >= 0 {
			key = unquoteJSON(object[i:keyEnd])
		}
		valueStart := skipJSONSpace(object, skipJSONSpace(object, keyEnd)+1)
		valueEnd := jsonValueEnd(object, valueStart)
		for p, want := range jsonLevelKeys {
			if strings.EqualFold(key, want) {
				values[p] = object[valueStart:valueEnd]
			}
		}
		if i = skipJSONSpace(object, valueEnd); object[i] == ',' {
			i = skipJSONSpace(object, i+1)
		}
	}
	for _, raw := range values {
		if level, ok := jsonLevelValue(raw); ok {
			return level, true
		}
	}
	return "", false
}

// jsonLevelValue accepts a recognized level string or an integer pino/bunyan level.
func jsonLevelValue(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	if raw[0] == '"' {
		text := raw[1 : len(raw)-1]
		if strings.IndexByte(text, '\\') >= 0 {
			text = unquoteJSON(raw)
		}
		return declaredLogLevel(text)
	}
	number, err := strconv.Atoi(raw)
	if err != nil {
		return "", false
	}
	level, ok := pinoLevels[number]
	return level, ok
}

// unquoteJSON decodes a valid JSON string literal that contains escapes. It is a
// separate function so the common unescaped path does not allocate.
func unquoteJSON(literal string) string {
	var text string
	_ = json.Unmarshal([]byte(literal), &text)
	return text
}

// jsonValueEnd returns the index just past the valid JSON value at s[i].
func jsonValueEnd(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '"':
			for j++; s[j] != '"'; j++ {
				if s[j] == '\\' {
					j++
				}
			}
			if depth == 0 {
				return j + 1
			}
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return j + 1
			}
			if depth < 0 {
				return j // a scalar ended at the enclosing object's closing brace
			}
		case ',', ' ', '\t', '\n', '\r':
			if depth == 0 {
				return j
			}
		}
	}
	return len(s)
}

func skipJSONSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	return i
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
