// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metricsaslogs serializes metrics into structured JSON lines suitable
// for writing to a log, letting other agent components piggyback metrics on
// their own logging (bypassing metric-tag cardinality limits) without
// shipping anything over the network themselves. Callers are responsible for
// actually writing the returned lines to their logger of choice, at
// whatever level they see fit.
package metricsaslogs

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

const (
	// MaxMetricsPerBatch is the maximum number of metrics packed into a
	// single serialized line before Serialize splits into more lines.
	MaxMetricsPerBatch = 500

	// DefaultMaxLineSizeBytes is the default value of Serialize's
	// maxLineSizeBytes parameter, mirroring the Logs intake's default
	// per-message size limit (logs_config.max_message_size_bytes). An
	// installation that configures a different limit, or a caller whose
	// logger adds its own framing around the line, should pass its actual
	// effective limit to Serialize instead of this default.
	DefaultMaxLineSizeBytes = constants.DefaultMaxMessageSizeBytes
)

// MetricType is the type of a metric passed to Serialize.
type MetricType string

const (
	// MetricTypeGauge is a gauge metric.
	MetricTypeGauge MetricType = "gauge"
	// MetricTypeCount is a count metric.
	MetricTypeCount MetricType = "count"
)

// Metric is a single metric passed to Serialize.
type Metric struct {
	Name  string            `json:"name"`
	Type  MetricType        `json:"type"`
	Value float64           `json:"value"`
	Tags  map[string]string `json:"tags,omitempty"`
}

// Serialize encodes metrics into one or more JSON lines, splitting into
// multiple lines whenever a batch would exceed MaxMetricsPerBatch or
// maxLineSizeBytes. maxLineSizeBytes should be the caller's actual
// effective per-message size limit (accounting for any framing the
// caller's own logger adds around the line), not necessarily
// DefaultMaxLineSizeBytes: an installation may configure a lower
// logs_config.max_message_size_bytes, and a line that overshoots the real
// limit gets truncated downstream, corrupting its JSON. Every line from
// one Serialize call carries the same "timestamp" field (Unix
// milliseconds, captured once at the start of the call), so metrics from
// one call keep an exact shared timestamp even if they end up split
// across lines. Returns nil for an empty input. A single metric that
// exceeds the byte limit on its own is still returned as its own line,
// since it can't be split any further.
func Serialize(metrics []Metric, maxLineSizeBytes int) ([]string, error) {
	if len(metrics) == 0 {
		return nil, nil
	}

	if maxLineSizeBytes <= 0 {
		return nil, fmt.Errorf("metricsaslogs: maxLineSizeBytes must be positive, got %d", maxLineSizeBytes)
	}

	for _, m := range metrics {
		if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
			return nil, fmt.Errorf("metricsaslogs: metric %q has non-finite value %v", m.Name, m.Value)
		}
	}

	// Captured once so every line from this call carries the exact same
	// timestamp.
	timestamp := float64(time.Now().UnixMilli())

	// emptyBatchOverhead is the encoded length of a line with zero metrics
	// for this call's timestamp — everything but the metric entries
	// themselves (the "message"/"timestamp" fields, and the object/array
	// punctuation). It's derived from marshalBatch itself, so it can't
	// drift out of sync with the actual wire format, and lets the loop
	// below compute a candidate batch's exact encoded size from its
	// metrics' individual entry lengths without re-marshaling the whole
	// batch on every append.
	emptyBatch, err := marshalBatch(nil, timestamp)
	if err != nil {
		return nil, err
	}
	emptyBatchOverhead := len(emptyBatch)

	// Accumulate metrics into a batch, flushing it as a line whenever
	// adding the next metric would push it past either limit. The
	// candidate batch's encoded size is derived from the running sum of
	// its metrics' own entry lengths (JSON array encoding is just
	// concatenation with commas), rather than by re-marshaling the whole
	// growing batch on every append, so this stays linear in the number of
	// metrics.
	var lines []string
	var batch []Metric
	entriesSize := 0
	for _, m := range metrics {
		entryLen, err := marshalEntryLen(m)
		if err != nil {
			return nil, err
		}

		if len(batch) > 0 {
			nextCount := len(batch) + 1
			// nextSize is emptyBatchOverhead (which already counts the 2
			// bytes for "[]") plus the entries' own bytes plus one comma
			// per entry after the first — going from an empty array to a
			// non-empty one doesn't add a second pair of brackets, it just
			// reuses the same 2 bytes and adds commas. For example, with
			// two 20-byte entries and emptyBatchOverhead 50 (for
			// `..."metrics":[]}`), the real line is
			// `..."metrics":[<20 bytes>,<20 bytes>]}`, i.e. 50 + 20 + 20 +
			// 1 (one comma) = 91 bytes — exactly emptyBatchOverhead +
			// entriesSize + (nextCount-1), with no further adjustment.
			nextSize := emptyBatchOverhead + entriesSize + entryLen + (nextCount - 1)
			if nextCount > MaxMetricsPerBatch || nextSize > maxLineSizeBytes {
				line, err := marshalBatchLine(batch, timestamp)
				if err != nil {
					return nil, err
				}
				lines = append(lines, line)
				batch = nil
				entriesSize = 0
			}
		}

		batch = append(batch, m)
		entriesSize += entryLen
	}

	line, err := marshalBatchLine(batch, timestamp)
	if err != nil {
		return nil, err
	}
	return append(lines, line), nil
}

// marshalEntryLen returns the encoded length of m's own JSON entry, as it
// would appear inside a batch's "metrics" array.
func marshalEntryLen(m Metric) (int, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func marshalBatch(ms []Metric, timestamp float64) ([]byte, error) {
	if ms == nil {
		// Force an empty "[]" rather than "null" so this always matches the
		// per-entry lengths marshalEntryLen computes, keeping
		// emptyBatchOverhead byte-exact.
		ms = []Metric{}
	}

	return json.Marshal(map[string]any{
		"message":   "agent metrics batch",
		"timestamp": timestamp,
		"metrics":   ms,
	})
}

func marshalBatchLine(ms []Metric, timestamp float64) (string, error) {
	data, err := marshalBatch(ms, timestamp)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
