// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package scraper provides a reusable OpenMetrics/Prometheus scraper that can
// be embedded by any Go core check that needs to collect metrics from a
// Prometheus-style endpoint.
package scraper

import (
	"errors"
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v2"
)

// Config holds all configuration for a single OpenMetrics scraper instance.
// Field names match the YAML keys used in conf.d/openmetrics.d/conf.yaml.
// Both V2 (openmetrics_endpoint) and V1 (prometheus_url) field names are
// supported; Resolve() merges V1 values into V2 fields.
type Config struct {
	// Endpoint
	OpenMetricsEndpoint string `yaml:"openmetrics_endpoint"`
	PrometheusURL       string `yaml:"prometheus_url"` // V1

	// Namespace prepended to every metric: "{namespace}.{metric_name}"
	Namespace string `yaml:"namespace"`

	// Metrics to collect. Each entry is either:
	//   - a string (regex pattern matching raw metric names)
	//   - a map[string]interface{} where key=raw_name, value=string(dd_name) or map with name/type
	Metrics      []interface{} `yaml:"metrics"`
	ExtraMetrics []interface{} `yaml:"extra_metrics"`

	// Prefix stripped from raw metric names before namespace is applied
	RawMetricPrefix  string `yaml:"raw_metric_prefix"`
	PromMetricPrefix string `yaml:"prometheus_metrics_prefix"` // V1

	// Type overrides: raw_metric_name → "counter"|"gauge"|"histogram"|"summary"|"rate"
	TypeOverrides map[string]string `yaml:"type_overrides"`

	// Label handling — V2 names
	RenameLabels  map[string]string            `yaml:"rename_labels"`
	ExcludeLabels []string                     `yaml:"exclude_labels"`
	IncludeLabels []string                     `yaml:"include_labels"`
	ShareLabels   map[string]ShareLabelsConfig `yaml:"share_labels"`

	// Label handling — V1 names
	LabelsMapper map[string]string           `yaml:"labels_mapper"`
	LabelJoins   map[string]LabelJoinsConfig `yaml:"label_joins"`

	// Hostname from labels
	HostnameLabel   string `yaml:"hostname_label"`
	HostnameFormat  string `yaml:"hostname_format"`
	LabelToHostname string `yaml:"label_to_hostname"` // V1

	// Metric filtering — V2
	ExcludeMetrics         []string            `yaml:"exclude_metrics"`
	ExcludeMetricsByLabels map[string][]string `yaml:"exclude_metrics_by_labels"`
	RawLineFilters         []string            `yaml:"raw_line_filters"`

	// Metric filtering — V1
	IgnoreMetrics         []string            `yaml:"ignore_metrics"`
	IgnoreMetricsByLabels map[string][]string `yaml:"ignore_metrics_by_labels"`

	// Histogram options — V2
	CollectHistogramBuckets          *bool `yaml:"collect_histogram_buckets"`
	HistogramBucketsAsDistributions  bool  `yaml:"histogram_buckets_as_distributions"`
	NonCumulativeHistogramBuckets    *bool `yaml:"non_cumulative_histogram_buckets"`
	CollectCountersWithDistributions bool  `yaml:"collect_counters_with_distributions"`

	// Histogram options — V1
	SendHistogramBuckets    *bool `yaml:"send_histograms_buckets"`
	SendDistributionBuckets bool  `yaml:"send_distribution_buckets"`

	// Counter options
	SendMonotonicCounter              *bool `yaml:"send_monotonic_counter"`
	SendMonotonicWithGauge            bool  `yaml:"send_monotonic_with_gauge"`
	SendDistributionCountsAsMonotonic bool  `yaml:"send_distribution_counts_as_monotonic"`
	SendDistributionSumsAsMonotonic   bool  `yaml:"send_distribution_sums_as_monotonic"`

	// Health & telemetry
	EnableHealthServiceCheck *bool `yaml:"enable_health_service_check"`
	HealthServiceCheck       *bool `yaml:"health_service_check"` // V1
	Telemetry                *bool `yaml:"telemetry"`
	IgnoreConnectionErrors   *bool `yaml:"ignore_connection_errors"`
	MaxReturnedMetrics       int   `yaml:"max_returned_metrics"`

	// HTTP options
	Timeout         int               `yaml:"timeout"`
	Headers         map[string]string `yaml:"headers"`
	ExtraHeaders    map[string]string `yaml:"extra_headers"`
	Username        string            `yaml:"username"`
	Password        string            `yaml:"password"`
	BearerTokenAuth bool              `yaml:"bearer_token_auth"`
	BearerTokenPath string            `yaml:"bearer_token_path"`

	// TLS
	TLSVerify     *bool  `yaml:"tls_verify"`
	TLSCert       string `yaml:"tls_cert"`
	TLSPrivateKey string `yaml:"tls_private_key"`
	TLSCACert     string `yaml:"tls_ca_cert"`

	// Proxy
	Proxy     map[string]string `yaml:"proxy"`
	SkipProxy bool              `yaml:"skip_proxy"`

	// Connection
	PersistConnections *bool `yaml:"persist_connections"`
	AllowRedirects     bool  `yaml:"allow_redirects"`

	// Cache
	CacheSharedLabels    *bool `yaml:"cache_shared_labels"`
	CacheMetricWildcards *bool `yaml:"cache_metric_wildcards"`

	// Tags
	TagByEndpoint *bool `yaml:"tag_by_endpoint"`

	// First scrape
	UseProcessStartTime *bool `yaml:"use_process_start_time"`
}

// ShareLabelsConfig configures label sharing from a source metric to target metrics.
type ShareLabelsConfig struct {
	Labels []string `yaml:"labels"`
	Match  []string `yaml:"match"`
	Values []string `yaml:"values"`
}

// LabelJoinsConfig configures V1-style label joins.
type LabelJoinsConfig struct {
	LabelsToMatch []string `yaml:"labels_to_match"`
	LabelsToGet   []string `yaml:"labels_to_get"`
}

// ParseConfig parses instance YAML bytes into a Config.
func ParseConfig(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse openmetrics instance config: %w", err)
	}
	cfg.Resolve()
	return cfg, nil
}

// Resolve merges V1 config field names into their V2 equivalents.
// V2 values take precedence when both are set.
func (c *Config) Resolve() {
	// Endpoint
	if c.OpenMetricsEndpoint == "" && c.PrometheusURL != "" {
		c.OpenMetricsEndpoint = c.PrometheusURL
	}

	// Metric prefix
	if c.RawMetricPrefix == "" && c.PromMetricPrefix != "" {
		c.RawMetricPrefix = c.PromMetricPrefix
	}

	// Label rename
	if len(c.RenameLabels) == 0 && len(c.LabelsMapper) > 0 {
		c.RenameLabels = c.LabelsMapper
	}

	// Label joins → share labels
	if len(c.ShareLabels) == 0 && len(c.LabelJoins) > 0 {
		c.ShareLabels = make(map[string]ShareLabelsConfig, len(c.LabelJoins))
		for metricName, lj := range c.LabelJoins {
			c.ShareLabels[metricName] = ShareLabelsConfig{
				Labels: lj.LabelsToGet,
				Match:  lj.LabelsToMatch,
			}
		}
	}

	// Hostname
	if c.HostnameLabel == "" && c.LabelToHostname != "" {
		c.HostnameLabel = c.LabelToHostname
	}

	// Metric exclusion
	if len(c.ExcludeMetrics) == 0 && len(c.IgnoreMetrics) > 0 {
		c.ExcludeMetrics = c.IgnoreMetrics
	}
	if len(c.ExcludeMetricsByLabels) == 0 && len(c.IgnoreMetricsByLabels) > 0 {
		c.ExcludeMetricsByLabels = c.IgnoreMetricsByLabels
	}

	// Histogram options
	if c.CollectHistogramBuckets == nil && c.SendHistogramBuckets != nil {
		c.CollectHistogramBuckets = c.SendHistogramBuckets
	}
	if !c.HistogramBucketsAsDistributions && c.SendDistributionBuckets {
		c.HistogramBucketsAsDistributions = true
	}

	// Health check
	if c.EnableHealthServiceCheck == nil && c.HealthServiceCheck != nil {
		c.EnableHealthServiceCheck = c.HealthServiceCheck
	}

	// Apply defaults
	c.applyDefaults()
}

func (c *Config) applyDefaults() {
	if c.CollectHistogramBuckets == nil {
		t := true
		c.CollectHistogramBuckets = &t
	}
	if c.EnableHealthServiceCheck == nil {
		t := true
		c.EnableHealthServiceCheck = &t
	}
	if c.Timeout == 0 {
		c.Timeout = 10
	}
	if c.CacheMetricWildcards == nil {
		t := true
		c.CacheMetricWildcards = &t
	}
	if c.CacheSharedLabels == nil {
		t := true
		c.CacheSharedLabels = &t
	}

	// Implied options
	if c.CollectCountersWithDistributions {
		c.HistogramBucketsAsDistributions = true
	}
	if c.HistogramBucketsAsDistributions {
		t := true
		c.CollectHistogramBuckets = &t
		if c.NonCumulativeHistogramBuckets == nil {
			c.NonCumulativeHistogramBuckets = &t
		}
	}
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if c.OpenMetricsEndpoint == "" {
		return errors.New("either `openmetrics_endpoint` or `prometheus_url` must be set")
	}
	if len(c.Metrics) == 0 && len(c.ExtraMetrics) == 0 {
		return errors.New("`metrics` must be set")
	}
	if c.HostnameLabel != "" && c.HostnameFormat != "" {
		if !strings.Contains(c.HostnameFormat, "<HOSTNAME>") {
			return errors.New("`hostname_format` must contain the placeholder `<HOSTNAME>`")
		}
	}
	return nil
}

// Endpoint returns the resolved endpoint URL.
func (c *Config) Endpoint() string {
	return c.OpenMetricsEndpoint
}
