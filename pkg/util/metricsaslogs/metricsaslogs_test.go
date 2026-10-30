// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricsaslogs

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerialize_SingleCallProducesOneLine(t *testing.T) {
	lines, err := Serialize([]Metric{
		{Name: "m1", Type: MetricTypeGauge, Value: 1, Tags: map[string]string{"a": "b"}},
		{Name: "m2", Type: MetricTypeCount, Value: 2, Tags: map[string]string{"c": "d"}},
	}, DefaultMaxLineSizeBytes)
	require.NoError(t, err)
	require.Len(t, lines, 1)

	var payload struct {
		Metrics []map[string]any `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &payload))
	assert.Len(t, payload.Metrics, 2)
}

func TestSerialize_EmptyBatchReturnsNoLines(t *testing.T) {
	lines, err := Serialize(nil, DefaultMaxLineSizeBytes)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

// countSerializedMetrics unmarshals every line's "metrics" array and returns
// the total number of metrics across all of them.
func countSerializedMetrics(t *testing.T, lines []string) int {
	total := 0
	for _, line := range lines {
		var payload struct {
			Metrics []map[string]any `json:"metrics"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &payload))
		total += len(payload.Metrics)
	}
	return total
}

func TestSerialize_SplitsWhenOverCountThreshold(t *testing.T) {
	n := MaxMetricsPerBatch + 1
	ms := make([]Metric, n)
	for i := range ms {
		ms[i] = Metric{Name: fmt.Sprintf("m%d", i), Type: MetricTypeGauge, Value: float64(i)}
	}

	lines, err := Serialize(ms, DefaultMaxLineSizeBytes)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, n, countSerializedMetrics(t, lines))
	assertSharedTimestamp(t, lines)
}

// lineTimestamp unmarshals one line's "timestamp" field.
func lineTimestamp(t *testing.T, line string) float64 {
	var payload struct {
		Timestamp float64 `json:"timestamp"`
	}
	require.NoError(t, json.Unmarshal([]byte(line), &payload))
	return payload.Timestamp
}

// assertSharedTimestamp asserts every line carries the same "timestamp"
// value.
func assertSharedTimestamp(t *testing.T, lines []string) {
	require.NotEmpty(t, lines)
	want := lineTimestamp(t, lines[0])
	assert.NotZero(t, want)
	for _, line := range lines[1:] {
		assert.Equal(t, want, lineTimestamp(t, line))
	}
}

func TestSerialize_SplitsWhenOverByteSizeLimit(t *testing.T) {
	bigTag := strings.Repeat("x", 300000)
	ms := []Metric{
		{Name: "m1", Type: MetricTypeGauge, Value: 1, Tags: map[string]string{"big": bigTag}},
		{Name: "m2", Type: MetricTypeGauge, Value: 2, Tags: map[string]string{"big": bigTag}},
		{Name: "m3", Type: MetricTypeGauge, Value: 3, Tags: map[string]string{"big": bigTag}},
		{Name: "m4", Type: MetricTypeGauge, Value: 4, Tags: map[string]string{"big": bigTag}},
	}

	lines, err := Serialize(ms, DefaultMaxLineSizeBytes)
	require.NoError(t, err)
	require.Greater(t, len(lines), 1)

	for _, line := range lines {
		assert.LessOrEqual(t, len(line), DefaultMaxLineSizeBytes)
	}
	assert.Equal(t, len(ms), countSerializedMetrics(t, lines))
}

func TestSerialize_SplitsWhenOverCustomByteSizeLimit(t *testing.T) {
	ms := []Metric{
		{Name: "m1", Type: MetricTypeGauge, Value: 1, Tags: map[string]string{"a": "aaaaaaaaaa"}},
		{Name: "m2", Type: MetricTypeGauge, Value: 2, Tags: map[string]string{"a": "aaaaaaaaaa"}},
		{Name: "m3", Type: MetricTypeGauge, Value: 3, Tags: map[string]string{"a": "aaaaaaaaaa"}},
	}
	const customLimit = 200

	lines, err := Serialize(ms, customLimit)
	require.NoError(t, err)
	require.Greater(t, len(lines), 1)

	for _, line := range lines {
		assert.LessOrEqual(t, len(line), customLimit)
	}
	assert.Equal(t, len(ms), countSerializedMetrics(t, lines))
}

func TestSerialize_ErrorsOnNonPositiveMaxLineSizeBytes(t *testing.T) {
	ms := []Metric{{Name: "m1", Type: MetricTypeGauge, Value: 1}}

	lines, err := Serialize(ms, 0)
	require.Error(t, err)
	assert.Empty(t, lines)

	lines, err = Serialize(ms, -1)
	require.Error(t, err)
	assert.Empty(t, lines)
}

func TestSerialize_ErrorsOnNonFiniteValue(t *testing.T) {
	lines, err := Serialize([]Metric{
		{Name: "m1", Type: MetricTypeGauge, Value: math.NaN()},
	}, DefaultMaxLineSizeBytes)
	require.Error(t, err)
	assert.Empty(t, lines)
}
