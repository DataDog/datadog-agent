// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetrics(t *testing.T) {
	mockOpenmetricsData := `
	grpc_server_msg_received_total{grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
	grpc_server_msg_received_total{grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 16631
	grpc_server_msg_sent_total{grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
	grpc_server_msg_sent_total{grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 72
	grpc_server_started_total{grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
	grpc_server_started_total{grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 16631
	`

	parsedMetrics, err := ParseMetrics([]byte(mockOpenmetricsData))
	require.NoError(t, err)

	expectedNumberOfMetrics := 6
	actualNumberOfMetrics := 0
	for _, fam := range parsedMetrics {
		actualNumberOfMetrics += len(fam.Samples)
	}

	assert.Equal(t, expectedNumberOfMetrics, actualNumberOfMetrics)
}

func TestParseMetricsWithFilter(t *testing.T) {
	testData := `# TYPE container_cpu_usage_seconds_total counter
container_cpu_usage_seconds_total{pod_name="test-pod",container="app"} 100
container_cpu_usage_seconds_total{pod_name="",container="empty"} 50
container_cpu_usage_seconds_total{pod_name="other-pod",container="sidecar"} 75
# TYPE container_memory_usage_bytes gauge
container_memory_usage_bytes{pod="test-pod",container="app"} 1000
container_memory_usage_bytes{pod="",container="empty"} 500
container_memory_usage_bytes{pod="other-pod",container="sidecar"} 750`

	t.Run("filter pod_name empty", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), []string{`pod_name=""`}, "")
		require.NoError(t, err)

		cpuFamily := findFamily(metrics, "container_cpu_usage_seconds")
		require.NotNil(t, cpuFamily)
		assert.Len(t, cpuFamily.Samples, 2, "should have 2 samples after filtering out pod_name=\"\"")
	})

	t.Run("filter pod empty", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), []string{`pod=""`}, "")
		require.NoError(t, err)

		memFamily := findFamily(metrics, "container_memory_usage_bytes")
		require.NotNil(t, memFamily)
		assert.Len(t, memFamily.Samples, 2, "should have 2 samples after filtering out pod=\"\"")
	})

	t.Run("filter both empty labels", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), []string{`pod_name=""`, `pod=""`}, "")
		require.NoError(t, err)

		cpuFamily := findFamily(metrics, "container_cpu_usage_seconds")
		require.NotNil(t, cpuFamily)
		assert.Len(t, cpuFamily.Samples, 2)

		memFamily := findFamily(metrics, "container_memory_usage_bytes")
		require.NotNil(t, memFamily)
		assert.Len(t, memFamily.Samples, 2)
	})

	t.Run("no filter", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), nil, "")
		require.NoError(t, err)

		cpuFamily := findFamily(metrics, "container_cpu_usage_seconds")
		require.NotNil(t, cpuFamily)
		assert.Len(t, cpuFamily.Samples, 3, "should have all 3 samples with no filter")
	})
}

func TestParseHistogramMetrics(t *testing.T) {
	testData := `# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 10
http_request_duration_seconds_bucket{le="0.5"} 25
http_request_duration_seconds_bucket{le="1"} 30
http_request_duration_seconds_bucket{le="+Inf"} 35
http_request_duration_seconds_sum 50.5
http_request_duration_seconds_count 35`

	metrics, err := ParseMetrics([]byte(testData))
	require.NoError(t, err)

	// All histogram components should be grouped under the same family
	histFamily := findFamily(metrics, "http_request_duration_seconds")
	require.NotNil(t, histFamily, "histogram family should exist")
	assert.Equal(t, "HISTOGRAM", histFamily.Type)
	assert.Len(t, histFamily.Samples, 6, "should have 4 buckets + sum + count")
}

func TestParseSummaryMetrics(t *testing.T) {
	testData := `# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.05
rpc_duration_seconds{quantile="0.9"} 0.08
rpc_duration_seconds{quantile="0.99"} 0.1
rpc_duration_seconds_sum 150.5
rpc_duration_seconds_count 1000`

	metrics, err := ParseMetrics([]byte(testData))
	require.NoError(t, err)

	summaryFamily := findFamily(metrics, "rpc_duration_seconds")
	require.NotNil(t, summaryFamily, "summary family should exist")
	assert.Equal(t, "SUMMARY", summaryFamily.Type)
	assert.Len(t, summaryFamily.Samples, 5, "should have 3 quantiles + sum + count")
}

func TestMetricTypeUppercase(t *testing.T) {
	testCases := []struct {
		name         string
		data         string
		expectedType string
	}{
		{
			name: "counter",
			data: `# TYPE http_requests_total counter
http_requests_total 100`,
			expectedType: "COUNTER",
		},
		{
			name: "gauge",
			data: `# TYPE temperature gauge
temperature 23.5`,
			expectedType: "GAUGE",
		},
		{
			name: "histogram",
			data: `# TYPE request_latency histogram
request_latency_bucket{le="0.1"} 10
request_latency_sum 5.5
request_latency_count 10`,
			expectedType: "HISTOGRAM",
		},
		{
			name: "summary",
			data: `# TYPE latency summary
latency{quantile="0.5"} 0.05
latency_sum 100
latency_count 200`,
			expectedType: "SUMMARY",
		},
		{
			name:         "untyped",
			data:         `some_metric 42`,
			expectedType: "UNTYPED",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metrics, err := ParseMetrics([]byte(tc.data))
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Equal(t, tc.expectedType, metrics[0].Type)
		})
	}
}

func TestWindowsLineEndings(t *testing.T) {
	// Data with Windows-style line endings (\r\n)
	testData := "# TYPE test_metric counter\r\ntest_metric{label=\"value\"} 42\r\ntest_metric{label=\"other\"} 100\r\n"

	metrics, err := ParseMetrics([]byte(testData))
	require.NoError(t, err)

	testFamily := findFamily(metrics, "test_metric")
	require.NotNil(t, testFamily)
	assert.Equal(t, "COUNTER", testFamily.Type)
	assert.Len(t, testFamily.Samples, 2)
}

func TestParseMetricsWithTimestamp(t *testing.T) {
	testData := `# TYPE test_metric gauge
test_metric{label="value"} 42 1609459200000`

	metrics, err := ParseMetrics([]byte(testData))
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].Samples, 1)

	// Timestamp should be set (1609459200000 ms = 2021-01-01 00:00:00 UTC)
	assert.NotZero(t, metrics[0].Samples[0].Timestamp)
}

func TestMetricNameLabel(t *testing.T) {
	// Test that __name__ label is correctly set
	testData := `grpc_server_handled_total{grpc_code="OK",grpc_method="PullImage"} 72`

	metrics, err := ParseMetrics([]byte(testData))
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].Samples, 1)

	sample := metrics[0].Samples[0]
	name, ok := sample.Metric["__name__"]
	assert.True(t, ok, "__name__ label should be present")
	assert.Equal(t, "grpc_server_handled_total", string(name))
}

func TestMetricsWithLeadingWhitespace(t *testing.T) {
	// Test that metrics with leading whitespace are parsed correctly
	// This is the format returned by containerd's /v1/metrics endpoint
	testData := `
				grpc_server_handled_total{grpc_code="InvalidArgument",grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 0
				grpc_server_handled_total{grpc_code="NotFound",grpc_method="PullImage",grpc_service="runtime.v1.ImageService",grpc_type="unary"} 0
				grpc_server_handled_total{grpc_code="NotFound",grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 16559
				grpc_server_handled_total{grpc_code="OK",grpc_method="PullImage",grpc_service="runtime.v1alpha2.ImageService",grpc_type="unary"} 72
			`

	metrics, err := ParseMetrics([]byte(testData))
	require.NoError(t, err)
	require.Len(t, metrics, 1, "should have 1 metric family")

	family := metrics[0]
	assert.Equal(t, "grpc_server_handled_total", family.Name)
	assert.Len(t, family.Samples, 4, "should have 4 samples")

	// Verify __name__ label is present in all samples
	for _, sample := range family.Samples {
		name, ok := sample.Metric["__name__"]
		assert.True(t, ok, "__name__ label should be present")
		assert.Equal(t, "grpc_server_handled_total", string(name))
	}
}

func TestParseMetricsToJSON(t *testing.T) {
	testData := `# TYPE http_requests_total counter
http_requests_total{method="GET",status="200"} 1234
http_requests_total{method="POST",status="500"} 5
# TYPE temperature gauge
temperature 23.5`

	jsonStr, err := ParseMetricsToJSON([]byte(testData), "")
	require.NoError(t, err)

	var families []MetricFamily
	err = json.Unmarshal([]byte(jsonStr), &families)
	require.NoError(t, err)

	require.Len(t, families, 2)
	assert.Equal(t, "http_requests", families[0].Name)
	assert.Equal(t, "COUNTER", families[0].Type)
	require.Len(t, families[0].Samples, 2)
	assert.Equal(t, 1234.0, families[0].Samples[0].Value)
	assert.Equal(t, "GET", families[0].Samples[0].Metric["method"])

	assert.Equal(t, "temperature", families[1].Name)
	assert.Equal(t, "GAUGE", families[1].Type)
	require.Len(t, families[1].Samples, 1)
	assert.Equal(t, 23.5, families[1].Samples[0].Value)
}

func TestParseMetricsToJSONEmpty(t *testing.T) {
	jsonStr, err := ParseMetricsToJSON([]byte(""), "")
	require.NoError(t, err)
	assert.Equal(t, "null", jsonStr)
}

func TestOpenMetricsCounterTotal(t *testing.T) {
	// OpenMetrics counters use _total suffix on series but the TYPE line uses the base name.
	// The parser should group foo_total series under the "foo" family.
	testData := `# TYPE foo counter
foo_total 17.0
foo_total{a="b"} 42.0
# EOF
`
	metrics, err := ParseMetricsWithFilter([]byte(testData), nil, "application/openmetrics-text")
	require.NoError(t, err)

	fooFamily := findFamily(metrics, "foo")
	require.NotNil(t, fooFamily, "should find family named 'foo'")
	assert.Equal(t, "COUNTER", fooFamily.Type)
	assert.Len(t, fooFamily.Samples, 2, "both foo_total series should be in the foo family")
}

func TestOpenMetricsGaugeAndHistogram(t *testing.T) {
	testData := `# TYPE temperature gauge
temperature 23.5
# TYPE request_latency histogram
request_latency_bucket{le="0.1"} 10
request_latency_bucket{le="+Inf"} 35
request_latency_sum 50.5
request_latency_count 35
# EOF
`
	metrics, err := ParseMetricsWithFilter([]byte(testData), nil, "application/openmetrics-text")
	require.NoError(t, err)

	tempFamily := findFamily(metrics, "temperature")
	require.NotNil(t, tempFamily)
	assert.Equal(t, "GAUGE", tempFamily.Type)
	assert.Len(t, tempFamily.Samples, 1)

	histFamily := findFamily(metrics, "request_latency")
	require.NotNil(t, histFamily)
	assert.Equal(t, "HISTOGRAM", histFamily.Type)
	assert.Len(t, histFamily.Samples, 4)
}

func TestOpenMetricsContentTypeSelection(t *testing.T) {
	// Same counter data, but with Prometheus content type should use PromParser.
	// The _total suffix is stripped from counter family names to match prometheus_client behavior.
	promData := `# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
`
	metrics, err := ParseMetricsWithFilter([]byte(promData), nil, "text/plain")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "http_requests", metrics[0].Name)
	assert.Equal(t, "COUNTER", metrics[0].Type)

	// Empty content type should default to Prometheus parser
	metrics2, err := ParseMetricsWithFilter([]byte(promData), nil, "")
	require.NoError(t, err)
	require.Len(t, metrics2, 1)
	assert.Equal(t, "http_requests", metrics2[0].Name)
}

func TestParseMetricsToJSONOpenMetrics(t *testing.T) {
	testData := `# TYPE http_requests counter
http_requests_total{method="GET"} 100
# EOF
`
	jsonStr, err := ParseMetricsToJSON([]byte(testData), "application/openmetrics-text; version=1.0.0")
	require.NoError(t, err)

	var families []MetricFamily
	err = json.Unmarshal([]byte(jsonStr), &families)
	require.NoError(t, err)
	require.Len(t, families, 1)
	assert.Equal(t, "http_requests", families[0].Name)
	assert.Equal(t, "COUNTER", families[0].Type)
}

func findFamily(families []MetricFamily, name string) *MetricFamily {
	for i := range families {
		if families[i].Name == name {
			return &families[i]
		}
	}
	return nil
}

// Benchmarks

func BenchmarkParseMetrics(b *testing.B) {
	var metrics []MetricFamily
	var err error

	data := generateLargeMetricsData()

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		metrics, err = ParseMetrics(data)
	}
	b.StopTimer()

	require.NoError(b, err)
	require.Len(b, metrics, 2)
	assert.Len(b, metrics[0].Samples, 1000)
	assert.Len(b, metrics[1].Samples, 1000)
}

func BenchmarkParseMetricsWithFilter(b *testing.B) {
	var metrics []MetricFamily
	var err error

	data := generateLargeMetricsDataWithEmptyPods()
	filter := []string{`pod_name=""`}

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		metrics, err = ParseMetricsWithFilter(data, filter, "")
	}
	b.StopTimer()

	require.NoError(b, err)
	require.Len(b, metrics, 1)
	assert.Len(b, metrics[0].Samples, 900)
}

func BenchmarkParseMetricsSmall(b *testing.B) {
	var metrics []MetricFamily
	var err error

	data := []byte(`# TYPE container_cpu_usage_seconds_total counter
container_cpu_usage_seconds_total{pod="pod-1",container="c1"} 100
container_cpu_usage_seconds_total{pod="pod-2",container="c2"} 200
container_cpu_usage_seconds_total{pod="pod-3",container="c3"} 300`)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		metrics, err = ParseMetrics(data)
	}
	b.StopTimer()

	require.NoError(b, err)
	require.Len(b, metrics, 1)
	assert.Len(b, metrics[0].Samples, 3)
}

func generateLargeMetricsData() []byte {
	var sb strings.Builder
	sb.WriteString("# TYPE container_cpu_usage_seconds_total counter\n")
	for i := range 1000 {
		fmt.Fprintf(&sb, "container_cpu_usage_seconds_total{pod=\"pod-%d\",container=\"c%d\",namespace=\"ns%d\"} %d\n", i, i%10, i%5, i*100)
	}
	sb.WriteString("# TYPE container_memory_usage_bytes gauge\n")
	for i := range 1000 {
		fmt.Fprintf(&sb, "container_memory_usage_bytes{pod=\"pod-%d\",container=\"c%d\",namespace=\"ns%d\"} %d\n", i, i%10, i%5, i*1024)
	}
	return []byte(sb.String())
}

func generateLargeMetricsDataWithEmptyPods() []byte {
	var sb strings.Builder
	sb.WriteString("# TYPE container_cpu_usage_seconds_total counter\n")
	for i := range 1000 {
		podName := fmt.Sprintf("pod-%d", i)
		if i%10 == 0 {
			podName = "" // 10% of metrics have empty pod_name
		}
		fmt.Fprintf(&sb, "container_cpu_usage_seconds_total{pod_name=\"%s\",container=\"c%d\"} %d\n", podName, i%10, i*100)
	}
	return []byte(sb.String())
}
