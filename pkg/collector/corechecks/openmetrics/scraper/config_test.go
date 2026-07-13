// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	yamlData := []byte(`
openmetrics_endpoint: http://localhost:9090/metrics
namespace: my_app
metrics:
  - go_goroutines
  - process_cpu_seconds_total
timeout: 15
`)

	cfg, err := ParseConfig(yamlData)
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:9090/metrics", cfg.OpenMetricsEndpoint)
	assert.Equal(t, "my_app", cfg.Namespace)
	assert.Len(t, cfg.Metrics, 2)
	assert.Equal(t, 15, cfg.Timeout)
}

func TestResolve_V1ToV2FieldMerging(t *testing.T) {
	t.Run("prometheus_url to openmetrics_endpoint", func(t *testing.T) {
		cfg := &Config{
			PrometheusURL: "http://localhost:9090/metrics",
		}
		cfg.Resolve()
		assert.Equal(t, "http://localhost:9090/metrics", cfg.OpenMetricsEndpoint)
	})

	t.Run("openmetrics_endpoint takes precedence over prometheus_url", func(t *testing.T) {
		cfg := &Config{
			OpenMetricsEndpoint: "http://localhost:8080/metrics",
			PrometheusURL:       "http://localhost:9090/metrics",
		}
		cfg.Resolve()
		assert.Equal(t, "http://localhost:8080/metrics", cfg.OpenMetricsEndpoint)
	})

	t.Run("labels_mapper to rename_labels", func(t *testing.T) {
		cfg := &Config{
			LabelsMapper: map[string]string{"old_label": "new_label"},
		}
		cfg.Resolve()
		assert.Equal(t, map[string]string{"old_label": "new_label"}, cfg.RenameLabels)
	})

	t.Run("rename_labels takes precedence over labels_mapper", func(t *testing.T) {
		cfg := &Config{
			RenameLabels: map[string]string{"v2_label": "v2_value"},
			LabelsMapper: map[string]string{"v1_label": "v1_value"},
		}
		cfg.Resolve()
		assert.Equal(t, map[string]string{"v2_label": "v2_value"}, cfg.RenameLabels)
	})

	t.Run("prometheus_metrics_prefix to raw_metric_prefix", func(t *testing.T) {
		cfg := &Config{
			PromMetricPrefix: "myapp_",
		}
		cfg.Resolve()
		assert.Equal(t, "myapp_", cfg.RawMetricPrefix)
	})

	t.Run("label_to_hostname to hostname_label", func(t *testing.T) {
		cfg := &Config{
			LabelToHostname: "node",
		}
		cfg.Resolve()
		assert.Equal(t, "node", cfg.HostnameLabel)
	})

	t.Run("ignore_metrics to exclude_metrics", func(t *testing.T) {
		cfg := &Config{
			IgnoreMetrics: []string{"metric_to_ignore"},
		}
		cfg.Resolve()
		assert.Equal(t, []string{"metric_to_ignore"}, cfg.ExcludeMetrics)
	})

	t.Run("ignore_metrics_by_labels to exclude_metrics_by_labels", func(t *testing.T) {
		cfg := &Config{
			IgnoreMetricsByLabels: map[string][]string{"label": {"val1", "val2"}},
		}
		cfg.Resolve()
		assert.Equal(t, map[string][]string{"label": {"val1", "val2"}}, cfg.ExcludeMetricsByLabels)
	})

	t.Run("label_joins to share_labels", func(t *testing.T) {
		cfg := &Config{
			LabelJoins: map[string]LabelJoinsConfig{
				"kube_pod_info": {
					LabelsToMatch: []string{"pod", "namespace"},
					LabelsToGet:   []string{"node", "host_ip"},
				},
			},
		}
		cfg.Resolve()
		require.Contains(t, cfg.ShareLabels, "kube_pod_info")
		assert.Equal(t, []string{"node", "host_ip"}, cfg.ShareLabels["kube_pod_info"].Labels)
		assert.Equal(t, []string{"pod", "namespace"}, cfg.ShareLabels["kube_pod_info"].Match)
	})

	t.Run("send_histograms_buckets to collect_histogram_buckets", func(t *testing.T) {
		f := false
		cfg := &Config{
			SendHistogramBuckets: &f,
		}
		cfg.Resolve()
		require.NotNil(t, cfg.CollectHistogramBuckets)
		assert.False(t, *cfg.CollectHistogramBuckets)
	})

	t.Run("send_distribution_buckets to histogram_buckets_as_distributions", func(t *testing.T) {
		cfg := &Config{
			SendDistributionBuckets: true,
		}
		cfg.Resolve()
		assert.True(t, cfg.HistogramBucketsAsDistributions)
	})

	t.Run("health_service_check to enable_health_service_check", func(t *testing.T) {
		f := false
		cfg := &Config{
			HealthServiceCheck: &f,
		}
		cfg.Resolve()
		require.NotNil(t, cfg.EnableHealthServiceCheck)
		assert.False(t, *cfg.EnableHealthServiceCheck)
	})
}

func TestValidate_MissingEndpoint(t *testing.T) {
	cfg := &Config{
		Metrics: []interface{}{"go_goroutines"},
	}
	cfg.Resolve()

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "openmetrics_endpoint")
}

func TestValidate_MissingMetrics(t *testing.T) {
	cfg := &Config{
		OpenMetricsEndpoint: "http://localhost:9090/metrics",
	}
	cfg.Resolve()

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics")
}

func TestValidate_HostnameFormatWithoutPlaceholder(t *testing.T) {
	cfg := &Config{
		OpenMetricsEndpoint: "http://localhost:9090/metrics",
		Metrics:             []interface{}{"go_goroutines"},
		HostnameLabel:       "node",
		HostnameFormat:      "prefix-missing-placeholder",
	}
	cfg.Resolve()

	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname_format")
	assert.Contains(t, err.Error(), "<HOSTNAME>")
}

func TestValidate_HostnameFormatWithPlaceholder(t *testing.T) {
	cfg := &Config{
		OpenMetricsEndpoint: "http://localhost:9090/metrics",
		Metrics:             []interface{}{"go_goroutines"},
		HostnameLabel:       "node",
		HostnameFormat:      "prefix-<HOSTNAME>-suffix",
	}
	cfg.Resolve()

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestDefaultValues(t *testing.T) {
	cfg := &Config{}
	cfg.Resolve()

	require.NotNil(t, cfg.CollectHistogramBuckets)
	assert.True(t, *cfg.CollectHistogramBuckets, "collect_histogram_buckets should default to true")

	require.NotNil(t, cfg.EnableHealthServiceCheck)
	assert.True(t, *cfg.EnableHealthServiceCheck, "enable_health_service_check should default to true")

	assert.Equal(t, 10, cfg.Timeout, "timeout should default to 10")

	require.NotNil(t, cfg.CacheMetricWildcards)
	assert.True(t, *cfg.CacheMetricWildcards, "cache_metric_wildcards should default to true")

	require.NotNil(t, cfg.CacheSharedLabels)
	assert.True(t, *cfg.CacheSharedLabels, "cache_shared_labels should default to true")
}

func TestImpliedOptions_CollectCountersWithDistributions(t *testing.T) {
	cfg := &Config{
		CollectCountersWithDistributions: true,
	}
	cfg.Resolve()

	assert.True(t, cfg.HistogramBucketsAsDistributions,
		"collect_counters_with_distributions should imply histogram_buckets_as_distributions=true")

	require.NotNil(t, cfg.CollectHistogramBuckets)
	assert.True(t, *cfg.CollectHistogramBuckets,
		"histogram_buckets_as_distributions should imply collect_histogram_buckets=true")

	require.NotNil(t, cfg.NonCumulativeHistogramBuckets)
	assert.True(t, *cfg.NonCumulativeHistogramBuckets,
		"histogram_buckets_as_distributions should imply non_cumulative_histogram_buckets=true when nil")
}

func TestImpliedOptions_HistogramBucketsAsDistributions(t *testing.T) {
	cfg := &Config{
		HistogramBucketsAsDistributions: true,
	}
	cfg.Resolve()

	require.NotNil(t, cfg.CollectHistogramBuckets)
	assert.True(t, *cfg.CollectHistogramBuckets,
		"histogram_buckets_as_distributions should force collect_histogram_buckets=true")

	require.NotNil(t, cfg.NonCumulativeHistogramBuckets)
	assert.True(t, *cfg.NonCumulativeHistogramBuckets,
		"histogram_buckets_as_distributions should default non_cumulative_histogram_buckets=true")
}

func TestImpliedOptions_NonCumulativePreservedWhenExplicit(t *testing.T) {
	f := false
	cfg := &Config{
		HistogramBucketsAsDistributions: true,
		NonCumulativeHistogramBuckets:   &f,
	}
	cfg.Resolve()

	require.NotNil(t, cfg.NonCumulativeHistogramBuckets)
	assert.False(t, *cfg.NonCumulativeHistogramBuckets,
		"explicit non_cumulative_histogram_buckets=false should be preserved")
}

func TestEndpoint(t *testing.T) {
	cfg := &Config{
		OpenMetricsEndpoint: "http://localhost:9090/metrics",
	}
	assert.Equal(t, "http://localhost:9090/metrics", cfg.Endpoint())
}
