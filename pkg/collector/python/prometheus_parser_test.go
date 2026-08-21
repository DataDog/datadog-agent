// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTextSimpleGauge(t *testing.T) {
	text := `# HELP go_goroutines Number of goroutines.
# TYPE go_goroutines gauge
go_goroutines 42
`
	result, err := parseText(text, "text/plain")
	require.NoError(t, err)
	require.NotEmpty(t, result)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	require.Len(t, families, 1)

	assert.Equal(t, "go_goroutines", families[0].Name)
	assert.Equal(t, "gauge", families[0].Type)
	assert.Equal(t, "Number of goroutines.", families[0].Help)
	require.Len(t, families[0].Samples, 1)
	assert.Equal(t, "go_goroutines", families[0].Samples[0].Name)
	assert.Equal(t, jsonFloat(42.0), families[0].Samples[0].Value)
	assert.Empty(t, families[0].Samples[0].Labels)
}

func TestParseTextCounter(t *testing.T) {
	text := `# HELP http_requests_total Total HTTP requests.
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 1027
http_requests_total{method="POST",code="200"} 42
`
	result, err := parseText(text, "text/plain")
	require.NoError(t, err)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	require.Len(t, families, 1)

	// Family name preserves the TYPE-line name verbatim; the Python
	// _json_to_metric function handles _total stripping.
	assert.Equal(t, "http_requests_total", families[0].Name)
	assert.Equal(t, "counter", families[0].Type)
	require.Len(t, families[0].Samples, 2)
	// Sample name retains the full _total suffix (used by histogram/summary transformers).
	assert.Equal(t, "http_requests_total", families[0].Samples[0].Name)
	assert.Equal(t, map[string]string{"method": "GET", "code": "200"}, families[0].Samples[0].Labels)
	assert.Equal(t, jsonFloat(1027.0), families[0].Samples[0].Value)
}

func TestParseTextHistogram(t *testing.T) {
	text := `# HELP http_request_duration_seconds Request duration.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 24054
http_request_duration_seconds_bucket{le="+Inf"} 144320
http_request_duration_seconds_sum 53423
http_request_duration_seconds_count 144320
`
	result, err := parseText(text, "text/plain")
	require.NoError(t, err)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	require.Len(t, families, 1)

	assert.Equal(t, "http_request_duration_seconds", families[0].Name)
	assert.Equal(t, "histogram", families[0].Type)
	require.Len(t, families[0].Samples, 4)
}

func TestParseTextMultipleFamilies(t *testing.T) {
	text := `# HELP metric_a A gauge.
# TYPE metric_a gauge
metric_a 1
# HELP metric_b A counter.
# TYPE metric_b counter
metric_b 2
`
	result, err := parseText(text, "text/plain")
	require.NoError(t, err)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	require.Len(t, families, 2)

	assert.Equal(t, "metric_a", families[0].Name)
	assert.Equal(t, "metric_b", families[1].Name)
}

func TestParseTextOpenMetrics(t *testing.T) {
	text := `# HELP go_goroutines Number of goroutines.
# TYPE go_goroutines gauge
go_goroutines 42
# EOF
`
	result, err := parseText(text, "application/openmetrics-text")
	require.NoError(t, err)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	require.Len(t, families, 1)
	assert.Equal(t, "go_goroutines", families[0].Name)
}

func TestParseTextEmpty(t *testing.T) {
	result, err := parseText("", "text/plain")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNewFeedFinishFlow(t *testing.T) {
	id := newPrometheusParser("text/plain")
	assert.Greater(t, id, int64(0))

	// Feed first family
	chunk1 := `# HELP metric_a A gauge.
# TYPE metric_a gauge
metric_a 1
# HELP metric_b A counter.
# TYPE metric_b counter
metric_b 2`

	result, err := feedPrometheusParser(id, chunk1)
	require.NoError(t, err)
	// Should have parsed metric_a (complete), metric_b buffered
	if result != "" {
		var families []jsonMetricFamily
		require.NoError(t, json.Unmarshal([]byte(result), &families))
		assert.Equal(t, "metric_a", families[0].Name)
	}

	// Finish to get remaining
	result, err = finishPrometheusParser(id)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	assert.Equal(t, "metric_b", families[0].Name)
}

func TestFeedSingleFamily(t *testing.T) {
	id := newPrometheusParser("text/plain")

	// Single family with no boundary - should buffer everything
	chunk := `# HELP metric_a A gauge.
# TYPE metric_a gauge
metric_a 1`

	result, err := feedPrometheusParser(id, chunk)
	require.NoError(t, err)
	assert.Empty(t, result, "single family should be buffered, not parsed")

	// Finish should parse it
	result, err = finishPrometheusParser(id)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	assert.Equal(t, "metric_a", families[0].Name)
}

func TestFeedHistogramNotSplitAcrossChunks(t *testing.T) {
	id := newPrometheusParser("text/plain")

	// TYPE + buckets in first chunk
	chunk1 := "# TYPE foo histogram\nfoo_bucket{le=\"1\"} 10\nfoo_bucket{le=\"2\"} 20"

	result, err := feedPrometheusParser(id, chunk1)
	require.NoError(t, err)
	assert.Empty(t, result)

	// _sum and _count arrive in a later chunk without a new TYPE line.
	// The fallback boundary detector must NOT split these from the histogram.
	chunk2 := "foo_sum 30\nfoo_count 2"

	result, err = feedPrometheusParser(id, chunk2)
	require.NoError(t, err)
	assert.Empty(t, result, "_sum and _count must not be split from the histogram family")

	// Finish returns the complete histogram
	result, err = finishPrometheusParser(id)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	var families []jsonMetricFamily
	require.NoError(t, json.Unmarshal([]byte(result), &families))
	require.Len(t, families, 1)
	assert.Equal(t, "foo", families[0].Name)
	assert.Equal(t, "histogram", families[0].Type)
	assert.Len(t, families[0].Samples, 4) // 2 buckets + sum + count
}

func TestFinishUnknownID(t *testing.T) {
	_, err := finishPrometheusParser(99999)
	assert.Error(t, err)
}
