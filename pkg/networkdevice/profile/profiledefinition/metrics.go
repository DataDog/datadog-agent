// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiledefinition

import (
	"regexp"
)

// ProfileMetricType metric type used to override default type of the metric
// By default metric type is derived from the type of the SNMP value, for example Counter32/64 -> rate.
type ProfileMetricType string

const (
	// ProfileMetricTypeGauge is used to create a gauge metric
	ProfileMetricTypeGauge ProfileMetricType = "gauge"

	// ProfileMetricTypeMonotonicCount is used to create a monotonic_count metric
	ProfileMetricTypeMonotonicCount ProfileMetricType = "monotonic_count"

	// ProfileMetricTypeMonotonicCountAndRate is used to create a monotonic_count and rate metric
	ProfileMetricTypeMonotonicCountAndRate ProfileMetricType = "monotonic_count_and_rate"

	// ProfileMetricTypeRate is used to create a rate metric
	ProfileMetricTypeRate ProfileMetricType = "rate"

	// ProfileMetricTypeFlagStream is used to create metric based on a value that represent flags
	// See details in https://github.com/DataDog/integrations-core/pull/7072
	ProfileMetricTypeFlagStream ProfileMetricType = "flag_stream"

	// ProfileMetricTypeCounter is DEPRECATED
	// `counter` is deprecated in favour of `rate`
	ProfileMetricTypeCounter ProfileMetricType = "counter"

	// ProfileMetricTypePercent is DEPRECATED
	// `percent` is deprecated in favour of `scale_factor`
	ProfileMetricTypePercent ProfileMetricType = "percent"
)

// SymbolConfig holds info for a single symbol/oid
type SymbolConfig struct {
	OID  string `yaml:"OID" json:"OID"`
	Name string `yaml:"name" json:"name"`

	ExtractValue         string         `yaml:"extract_value" json:"extract_value"`
	ExtractValueCompiled *regexp.Regexp // TODO: ADD `-` yaml/json annotation?

	MatchPattern         string         `yaml:"match_pattern" json:"match_pattern"`
	MatchValue           string         `yaml:"match_value" json:"match_value"`
	MatchPatternCompiled *regexp.Regexp // TODO: ADD `-` yaml/json annotation?

	ScaleFactor      float64 `yaml:"scale_factor" json:"scale_factor"`
	Format           string  `yaml:"format" json:"format"`
	ConstantValueOne bool    `yaml:"constant_value_one" json:"constant_value_one"`

	// `metric_type` is used for force the metric type
	//   When empty, by default, the metric type is derived from SNMP OID value type.
	//   Valid `metric_type` types: `gauge`, `rate`, `monotonic_count`, `monotonic_count_and_rate`
	//   Deprecated types: `counter` (use `rate` instead), percent (use `scale_factor` instead)
	MetricType ProfileMetricType `yaml:"metric_type" json:"metric_type"`
}

// MetricTagConfig holds metric tag info
type MetricTagConfig struct {
	Tag string `yaml:"tag" json:"tag"`

	// Table config
	Index uint `yaml:"index" json:"index"`

	// TODO: refactor to rename to `symbol` instead (keep backward compat with `column`)
	Column SymbolConfig `yaml:"column" json:"column"`

	// Symbol config
	OID  string `yaml:"OID" json:"OID"`
	Name string `yaml:"symbol" json:"symbol"`

	IndexTransform []MetricIndexTransform `yaml:"index_transform" json:"index_transform"`

	Mapping map[string]string `yaml:"mapping" json:"mapping"`

	// Regex
	Match string            `yaml:"match" json:"match"`
	Tags  map[string]string `yaml:"tags" json:"tags"`

	// TODO: ADD `-` yaml/json annotation?
	SymbolTag string
	Pattern   *regexp.Regexp
}

// MetricTagConfigList holds configs for a list of metric tags
type MetricTagConfigList []MetricTagConfig

// MetricIndexTransform holds configs for metric index transform
type MetricIndexTransform struct {
	Start uint `yaml:"start" json:"start"`
	End   uint `yaml:"end" json:"end"`
}

// MetricsConfigOption holds config for metrics options
type MetricsConfigOption struct {
	Placement    uint   `yaml:"placement" json:"placement"`
	MetricSuffix string `yaml:"metric_suffix" json:"metric_suffix"`
}

// MetricsConfig holds configs for a metric
type MetricsConfig struct {
	// Symbol configs
	Symbol SymbolConfig `yaml:"symbol" json:"symbol"`

	// Legacy Symbol configs syntax
	OID  string `yaml:"OID" json:"OID"`
	Name string `yaml:"name" json:"name"`

	// Table configs
	Symbols []SymbolConfig `yaml:"symbols" json:"symbols"`

	StaticTags []string            `yaml:"static_tags" json:"static_tags"`
	MetricTags MetricTagConfigList `yaml:"metric_tags" json:"metric_tags"`

	ForcedType ProfileMetricType `yaml:"forced_type" json:"forced_type"` // deprecated in favour of metric_type
	MetricType ProfileMetricType `yaml:"metric_type" json:"metric_type"`

	Options MetricsConfigOption `yaml:"options" json:"options"`
}

// GetSymbolTags returns symbol tags
func (m *MetricsConfig) GetSymbolTags() []string {
	var symbolTags []string
	for _, metricTag := range m.MetricTags {
		symbolTags = append(symbolTags, metricTag.SymbolTag)
	}
	return symbolTags
}

// IsColumn returns true if the metrics config define columns metrics
func (m *MetricsConfig) IsColumn() bool {
	return len(m.Symbols) > 0
}

// IsScalar returns true if the metrics config define scalar metrics
func (m *MetricsConfig) IsScalar() bool {
	return m.Symbol.OID != "" && m.Symbol.Name != ""
}

// NormalizeMetrics converts legacy syntax to new syntax
// 1/ converts old symbol syntax to new symbol syntax
// metric.Name and metric.OID info are moved to metric.Symbol.Name and metric.Symbol.OID
func NormalizeMetrics(metrics []MetricsConfig) {
	for i := range metrics {
		metric := &metrics[i]

		// converts old symbol syntax to new symbol syntax
		if metric.Symbol.Name == "" && metric.Symbol.OID == "" && metric.Name != "" && metric.OID != "" {
			metric.Symbol.Name = metric.Name
			metric.Symbol.OID = metric.OID
			metric.Name = ""
			metric.OID = ""
		}
	}
}
