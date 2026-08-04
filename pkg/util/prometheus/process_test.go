// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheus

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessMetrics_BasicGauge(t *testing.T) {
	data := []byte(`# TYPE temperature gauge
temperature{location="us-east",host="web01"} 72.5
temperature{location="us-west",host="web02"} 68.3`)

	cfg := &ProcessConfig{
		StaticTags: []string{"env:prod"},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "temperature", result[0].Name)
	assert.Equal(t, "GAUGE", result[0].Type)
	require.Len(t, result[0].Samples, 2)

	// Check that tags are built and static tags appended
	sample := result[0].Samples[0]
	assert.Equal(t, 72.5, sample.Value)
	assert.Contains(t, sample.Tags, "env:prod")
	assert.Contains(t, sample.Tags, "location:us-east")
	assert.Contains(t, sample.Tags, "host:web01")
}

func TestProcessMetrics_ExcludeLabels(t *testing.T) {
	data := []byte(`# TYPE cpu gauge
cpu{host="web01",pod="abc",container_id="xyz"} 0.5`)

	cfg := &ProcessConfig{
		ExcludeLabels: []string{"container_id"},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Samples, 1)

	tags := result[0].Samples[0].Tags
	assert.Contains(t, tags, "host:web01")
	assert.Contains(t, tags, "pod:abc")
	for _, tag := range tags {
		assert.NotContains(t, tag, "container_id")
	}
}

func TestProcessMetrics_IncludeLabels(t *testing.T) {
	data := []byte(`# TYPE cpu gauge
cpu{host="web01",pod="abc",container_id="xyz"} 0.5`)

	cfg := &ProcessConfig{
		IncludeLabels: []string{"host"},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result[0].Samples, 1)

	tags := result[0].Samples[0].Tags
	assert.Contains(t, tags, "host:web01")
	assert.Len(t, tags, 1) // only "host" included, no static tags
}

func TestProcessMetrics_RenameLabels(t *testing.T) {
	data := []byte(`# TYPE cpu gauge
cpu{old_name="value1"} 1.0`)

	cfg := &ProcessConfig{
		RenameLabels: map[string]string{"old_name": "new_name"},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)

	tags := result[0].Samples[0].Tags
	assert.Contains(t, tags, "new_name:value1")
	for _, tag := range tags {
		assert.NotContains(t, tag, "old_name:")
	}
}

func TestProcessMetrics_ExcludeMetrics(t *testing.T) {
	data := []byte(`# TYPE wanted gauge
wanted{a="1"} 1.0
# TYPE unwanted gauge
unwanted{a="2"} 2.0
# TYPE also_unwanted gauge
also_unwanted{a="3"} 3.0`)

	cfg := &ProcessConfig{
		ExcludeMetrics:         []string{"unwanted"},
		ExcludeMetricsPatterns: []string{"also_.*"},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "wanted", result[0].Name)
}

func TestProcessMetrics_ExcludeMetricsByLabels(t *testing.T) {
	data := []byte(`# TYPE http_requests gauge
http_requests{status="200"} 100
http_requests{status="500"} 5
http_requests{status="503"} 2`)

	cfg := &ProcessConfig{
		ExcludeMetricsByLabels: map[string][]string{
			"status": {"5.."},
		},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Samples, 1)
	assert.Equal(t, 100.0, result[0].Samples[0].Value)
}

func TestProcessMetrics_ExcludeMetricsByLabelsAnyValue(t *testing.T) {
	data := []byte(`# TYPE http_requests gauge
http_requests{status="200"} 100
http_requests{debug="true"} 5`)

	cfg := &ProcessConfig{
		ExcludeMetricsByLabels: map[string][]string{
			"debug": {},
		},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Samples, 1)
	assert.Equal(t, 100.0, result[0].Samples[0].Value)
}

func TestProcessMetrics_RawMetricPrefix(t *testing.T) {
	data := []byte(`# TYPE myapp_requests gauge
myapp_requests{a="1"} 10`)

	cfg := &ProcessConfig{
		RawMetricPrefix: "myapp_",
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "requests", result[0].Name)
}

func TestProcessMetrics_HostnameLabel(t *testing.T) {
	data := []byte(`# TYPE cpu gauge
cpu{node="web01.example.com",region="us-east"} 0.5`)

	cfg := &ProcessConfig{
		HostnameLabel: "node",
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	assert.Equal(t, "web01.example.com", result[0].Samples[0].Hostname)
}

func TestProcessMetrics_HostnameFormat(t *testing.T) {
	data := []byte(`# TYPE cpu gauge
cpu{node="web01"} 0.5`)

	cfg := &ProcessConfig{
		HostnameLabel:  "node",
		HostnameFormat: "<HOSTNAME>.example.com",
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	assert.Equal(t, "web01.example.com", result[0].Samples[0].Hostname)
}

func TestProcessMetrics_HistogramNormalization(t *testing.T) {
	data := []byte(`# TYPE http_duration histogram
http_duration_bucket{le="0.1"} 10
http_duration_bucket{le="0.5"} 50
http_duration_bucket{le="+Inf"} 100
http_duration_sum 35.2
http_duration_count 100`)

	cfg := &ProcessConfig{}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "HISTOGRAM", result[0].Type)

	// le should be renamed to upper_bound
	for _, s := range result[0].Samples {
		for _, tag := range s.Tags {
			assert.NotContains(t, tag, "le:")
		}
		if s.SampleName == "http_duration_bucket" {
			hasUpperBound := false
			for _, tag := range s.Tags {
				if tag == "upper_bound:0.1" || tag == "upper_bound:0.5" || tag == "upper_bound:+Inf" {
					hasUpperBound = true
				}
			}
			assert.True(t, hasUpperBound, "bucket sample should have upper_bound tag, got: %v", s.Tags)
		}
	}
}

func TestProcessMetrics_SummaryNormalization(t *testing.T) {
	data := []byte(`# TYPE rpc_duration summary
rpc_duration{quantile="0.50"} 0.5
rpc_duration{quantile="0.90"} 0.9
rpc_duration{quantile="0.990"} 0.99
rpc_duration_sum 100
rpc_duration_count 200`)

	cfg := &ProcessConfig{}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// quantile values should be canonicalized
	for _, s := range result[0].Samples {
		if s.SampleName == "rpc_duration" {
			for _, tag := range s.Tags {
				// "0.50" → "0.5", "0.990" → "0.99"
				assert.NotEqual(t, "quantile:0.50", tag)
				assert.NotEqual(t, "quantile:0.990", tag)
			}
		}
	}
}

func TestProcessMetrics_ShareLabelsUnconditional(t *testing.T) {
	data := []byte(`# TYPE kube_pod_info gauge
kube_pod_info{pod="pod1",namespace="ns1",node="node1",host_ip="10.0.0.1"} 1
# TYPE kube_pod_status gauge
kube_pod_status{pod="pod1",namespace="ns1"} 1`)

	cfg := &ProcessConfig{
		ShareLabels: map[string]ShareLabelConfig{
			"kube_pod_info": {
				Labels: []string{"node", "host_ip"},
				Values: []float64{1},
			},
		},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)

	// Find kube_pod_status
	var statusFamily *ProcessedMetricFamily
	for i := range result {
		if result[i].Name == "kube_pod_status" {
			statusFamily = &result[i]
			break
		}
	}
	require.NotNil(t, statusFamily)
	require.Len(t, statusFamily.Samples, 1)

	tags := statusFamily.Samples[0].Tags
	assert.Contains(t, tags, "node:node1")
	assert.Contains(t, tags, "host_ip:10.0.0.1")
}

func TestProcessMetrics_ShareLabelsConditional(t *testing.T) {
	data := []byte(`# TYPE kube_pod_info gauge
kube_pod_info{pod="pod1",namespace="ns1",node="node1"} 1
kube_pod_info{pod="pod2",namespace="ns2",node="node2"} 1
# TYPE kube_pod_status gauge
kube_pod_status{pod="pod1",namespace="ns1"} 1
kube_pod_status{pod="pod2",namespace="ns2"} 2`)

	cfg := &ProcessConfig{
		ShareLabels: map[string]ShareLabelConfig{
			"kube_pod_info": {
				Match:  []string{"pod", "namespace"},
				Labels: []string{"node"},
				Values: []float64{1},
			},
		},
	}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)

	var statusFamily *ProcessedMetricFamily
	for i := range result {
		if result[i].Name == "kube_pod_status" {
			statusFamily = &result[i]
			break
		}
	}
	require.NotNil(t, statusFamily)
	require.Len(t, statusFamily.Samples, 2)

	// pod1 should get node1, pod2 should get node2
	for _, s := range statusFamily.Samples {
		hasPod1 := false
		hasPod2 := false
		for _, tag := range s.Tags {
			if tag == "pod:pod1" {
				hasPod1 = true
			}
			if tag == "pod:pod2" {
				hasPod2 = true
			}
		}
		if hasPod1 {
			assert.Contains(t, s.Tags, "node:node1")
		}
		if hasPod2 {
			assert.Contains(t, s.Tags, "node:node2")
		}
	}
}

func TestProcessMetrics_NaNInfSkipped(t *testing.T) {
	data := []byte(`# TYPE test gauge
test{a="1"} 1.0
test{a="2"} NaN
test{a="3"} +Inf`)

	cfg := &ProcessConfig{}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Samples, 1)
	assert.Equal(t, 1.0, result[0].Samples[0].Value)
}

func TestProcessMetrics_OpenMetricsContentType(t *testing.T) {
	data := []byte(`# TYPE http_requests counter
http_requests_total{method="GET"} 100
# EOF
`)

	cfg := &ProcessConfig{}

	result, err := ProcessMetrics(data, "application/openmetrics-text; version=1.0.0", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "http_requests", result[0].Name)
	assert.Equal(t, "COUNTER", result[0].Type)
}

func TestProcessMetrics_PrometheusCounterTotalStripped(t *testing.T) {
	data := []byte(`# TYPE http_requests_total counter
http_requests_total{method="GET"} 100`)

	cfg := &ProcessConfig{}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "http_requests", result[0].Name) // _total stripped for Prometheus counters too
	assert.Equal(t, "COUNTER", result[0].Type)
}

func TestProcessMetrics_NameLabelExcluded(t *testing.T) {
	data := []byte(`# TYPE test gauge
test{a="1"} 1.0`)

	cfg := &ProcessConfig{}

	result, err := ProcessMetrics(data, "", cfg)
	require.NoError(t, err)

	// __name__ should not appear in tags
	for _, tag := range result[0].Samples[0].Tags {
		assert.NotContains(t, tag, "__name__")
	}
}

func TestProcessMetricsToJSON(t *testing.T) {
	data := []byte(`# TYPE cpu gauge
cpu{host="web01"} 0.5`)

	configJSON := `{"static_tags":["env:test"],"exclude_labels":["__name__"]}`

	jsonResult, err := ProcessMetricsToJSON(data, "", configJSON)
	require.NoError(t, err)

	var families []ProcessedMetricFamily
	err = json.Unmarshal([]byte(jsonResult), &families)
	require.NoError(t, err)
	require.Len(t, families, 1)
	assert.Equal(t, "cpu", families[0].Name)
	assert.Contains(t, families[0].Samples[0].Tags, "env:test")
	assert.Contains(t, families[0].Samples[0].Tags, "host:web01")
}

func TestProcessMetricsToJSON_InvalidConfig(t *testing.T) {
	_, err := ProcessMetricsToJSON([]byte(""), "", `{invalid`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config")
}

func TestCanonicalizeNumericLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0.5", "0.5"},
		{"0.50", "0.5"},
		{"0.990", "0.99"},
		{"1", "1"},
		{"0.0", "0"},
		{"-0.0", "0"},
		{"+Inf", "+Inf"},
		{"-Inf", "-Inf"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, canonicalizeNumericLabel(tt.input))
		})
	}
}

// Benchmarks

func BenchmarkProcessMetrics(b *testing.B) {
	data := generateLargeMetricsData()
	cfg := &ProcessConfig{
		ExcludeLabels: []string{"container_id"},
		RenameLabels:  map[string]string{"pod_name": "pod"},
		StaticTags:    []string{"env:prod", "endpoint:http://localhost:9090/metrics"},
	}

	var result []ProcessedMetricFamily
	var err error

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		result, err = ProcessMetrics(data, "", cfg)
	}
	b.StopTimer()

	require.NoError(b, err)
	require.NotEmpty(b, result)
}

func BenchmarkProcessMetricsWithShareLabels(b *testing.B) {
	// Build data with a source metric and many target metrics
	var lines []byte
	lines = append(lines, []byte("# TYPE kube_pod_info gauge\n")...)
	for i := 0; i < 100; i++ {
		line := []byte(fmt.Sprintf("kube_pod_info{pod=\"pod-%d\",namespace=\"ns1\",node=\"node-%d\"} 1\n", i, i%10))
		lines = append(lines, line...)
	}
	lines = append(lines, []byte("# TYPE kube_pod_status gauge\n")...)
	for i := 0; i < 1000; i++ {
		line := []byte(fmt.Sprintf("kube_pod_status{pod=\"pod-%d\",namespace=\"ns1\"} %d\n", i%100, i))
		lines = append(lines, line...)
	}

	cfg := &ProcessConfig{
		ShareLabels: map[string]ShareLabelConfig{
			"kube_pod_info": {
				Match:  []string{"pod", "namespace"},
				Labels: []string{"node"},
				Values: []float64{1},
			},
		},
		StaticTags: []string{"env:prod"},
	}

	var result []ProcessedMetricFamily
	var err error

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		result, err = ProcessMetrics(lines, "", cfg)
	}
	b.StopTimer()

	require.NoError(b, err)
	require.NotEmpty(b, result)
}
