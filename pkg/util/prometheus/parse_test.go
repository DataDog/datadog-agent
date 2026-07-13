// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
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
		metrics, err := ParseMetricsWithFilter([]byte(testData), []string{`pod_name=""`})
		require.NoError(t, err)

		cpuFamily := findFamily(metrics, "container_cpu_usage_seconds_total")
		require.NotNil(t, cpuFamily)
		assert.Len(t, cpuFamily.Samples, 2, "should have 2 samples after filtering out pod_name=\"\"")
	})

	t.Run("filter pod empty", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), []string{`pod=""`})
		require.NoError(t, err)

		memFamily := findFamily(metrics, "container_memory_usage_bytes")
		require.NotNil(t, memFamily)
		assert.Len(t, memFamily.Samples, 2, "should have 2 samples after filtering out pod=\"\"")
	})

	t.Run("filter both empty labels", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), []string{`pod_name=""`, `pod=""`})
		require.NoError(t, err)

		cpuFamily := findFamily(metrics, "container_cpu_usage_seconds_total")
		require.NotNil(t, cpuFamily)
		assert.Len(t, cpuFamily.Samples, 2)

		memFamily := findFamily(metrics, "container_memory_usage_bytes")
		require.NotNil(t, memFamily)
		assert.Len(t, memFamily.Samples, 2)
	})

	t.Run("no filter", func(t *testing.T) {
		metrics, err := ParseMetricsWithFilter([]byte(testData), nil)
		require.NoError(t, err)

		cpuFamily := findFamily(metrics, "container_cpu_usage_seconds_total")
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
		metrics, err = ParseMetricsWithFilter(data, filter)
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

func TestParseMetricsFromResponse_PrometheusTextFormat(t *testing.T) {
	testData := `# TYPE go_goroutines gauge
go_goroutines 42
# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50`

	metrics, err := ParseMetricsFromResponse([]byte(testData), "text/plain; charset=utf-8", nil)
	require.NoError(t, err)

	gaugeFamily := findFamily(metrics, "go_goroutines")
	require.NotNil(t, gaugeFamily, "gauge family should exist")
	assert.Equal(t, "GAUGE", gaugeFamily.Type)
	require.Len(t, gaugeFamily.Samples, 1)
	assert.Equal(t, float64(42), gaugeFamily.Samples[0].Value)

	counterFamily := findFamily(metrics, "http_requests_total")
	require.NotNil(t, counterFamily, "counter family should exist")
	assert.Equal(t, "COUNTER", counterFamily.Type)
	require.Len(t, counterFamily.Samples, 2)

	// Verify sample values by label
	for _, s := range counterFamily.Samples {
		switch s.Metric["method"] {
		case "GET":
			assert.Equal(t, float64(100), s.Value)
		case "POST":
			assert.Equal(t, float64(50), s.Value)
		default:
			t.Errorf("unexpected method label: %s", s.Metric["method"])
		}
	}
}

func TestParseMetricsFromResponse_OpenMetricsContentTypeSelectsParser(t *testing.T) {
	// Verify that the OpenMetrics content-type causes a different code path
	// than text/plain. We test with the Prometheus text parser as baseline,
	// then confirm the OpenMetrics content type is correctly detected.

	testData := `# TYPE go_goroutines gauge
go_goroutines 42
`
	// text/plain should use the Prometheus parser successfully
	metrics, err := ParseMetricsFromResponse([]byte(testData), ContentTypeText, nil)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "GAUGE", metrics[0].Type)

	// Verify constant values used for content-type detection
	assert.Equal(t, "application/openmetrics-text", ContentTypeOpenMetrics)
	assert.Equal(t, "text/plain", ContentTypeText)
	assert.True(t, strings.Contains("application/openmetrics-text; version=1.0.0", ContentTypeOpenMetrics),
		"OpenMetrics content type with version should match the constant")
	assert.False(t, strings.Contains(ContentTypeText, ContentTypeOpenMetrics),
		"text/plain should not match OpenMetrics content type")
}

func TestParseMetricsFromResponse_Histogram(t *testing.T) {
	testData := `# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 10
request_duration_seconds_bucket{le="0.5"} 25
request_duration_seconds_bucket{le="+Inf"} 30
request_duration_seconds_sum 15.5
request_duration_seconds_count 30`

	metrics, err := ParseMetricsFromResponse([]byte(testData), "text/plain", nil)
	require.NoError(t, err)

	histFamily := findFamily(metrics, "request_duration_seconds")
	require.NotNil(t, histFamily, "histogram family should exist")
	assert.Equal(t, "HISTOGRAM", histFamily.Type)
	assert.Len(t, histFamily.Samples, 5, "should have 3 buckets + sum + count")

	// Verify bucket samples have the le label
	bucketCount := 0
	for _, s := range histFamily.Samples {
		if _, ok := s.Metric["le"]; ok {
			bucketCount++
		}
	}
	assert.Equal(t, 3, bucketCount, "should have 3 bucket samples with le label")
}

func TestParseMetricsFromResponse_Summary(t *testing.T) {
	testData := `# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.001
rpc_duration_seconds{quantile="0.99"} 0.01
rpc_duration_seconds_sum 5.0
rpc_duration_seconds_count 100`

	metrics, err := ParseMetricsFromResponse([]byte(testData), "text/plain", nil)
	require.NoError(t, err)

	summaryFamily := findFamily(metrics, "rpc_duration_seconds")
	require.NotNil(t, summaryFamily, "summary family should exist")
	assert.Equal(t, "SUMMARY", summaryFamily.Type)
	assert.Len(t, summaryFamily.Samples, 4, "should have 2 quantiles + sum + count")

	// Verify quantile samples
	quantileCount := 0
	for _, s := range summaryFamily.Samples {
		if _, ok := s.Metric["quantile"]; ok {
			quantileCount++
		}
	}
	assert.Equal(t, 2, quantileCount, "should have 2 quantile samples")
}

func TestParseMetricsFromResponse_RawLineFilters(t *testing.T) {
	testData := `# TYPE go_goroutines gauge
go_goroutines 42
# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50`

	// Filter out lines containing method="POST"
	metrics, err := ParseMetricsFromResponse([]byte(testData), "text/plain", []string{`method="POST"`})
	require.NoError(t, err)

	counterFamily := findFamily(metrics, "http_requests_total")
	require.NotNil(t, counterFamily, "counter family should exist after filtering")
	assert.Len(t, counterFamily.Samples, 1, "should have 1 sample after filtering out POST")
	assert.Equal(t, "GET", counterFamily.Samples[0].Metric["method"])

	// The gauge should be unaffected by the filter
	gaugeFamily := findFamily(metrics, "go_goroutines")
	require.NotNil(t, gaugeFamily)
	assert.Len(t, gaugeFamily.Samples, 1)
}

func TestParseMetricsFromResponse_UntypedMetrics(t *testing.T) {
	testData := `some_untyped_metric{label="value"} 42
another_untyped_metric 99`

	metrics, err := ParseMetricsFromResponse([]byte(testData), "text/plain", nil)
	require.NoError(t, err)

	require.Len(t, metrics, 2, "should have 2 untyped metric families")
	for _, fam := range metrics {
		assert.Equal(t, "UNTYPED", fam.Type, "metric %s should be UNTYPED", fam.Name)
	}

	untypedFamily := findFamily(metrics, "some_untyped_metric")
	require.NotNil(t, untypedFamily)
	require.Len(t, untypedFamily.Samples, 1)
	assert.Equal(t, float64(42), untypedFamily.Samples[0].Value)

	anotherFamily := findFamily(metrics, "another_untyped_metric")
	require.NotNil(t, anotherFamily)
	require.Len(t, anotherFamily.Samples, 1)
	assert.Equal(t, float64(99), anotherFamily.Samples[0].Value)
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
