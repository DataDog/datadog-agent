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
	ProfileMetricTypeUnset ProfileMetricType = ""

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

// SymbolConfigCompat is used to deserialize string field or SymbolConfig.
// For OID/Name to Symbol harmonization:
// When users declare metric tag like:
//
//	metric_tags:
//	  - OID: 1.2.3
//	    symbol: aSymbol
//
// this will lead to OID stored as MetricTagConfig.OID  and name stored as MetricTagConfig.Symbol.Name
// When this happens, in ValidateEnrichMetricTags we harmonize by moving MetricTagConfig.OID to MetricTagConfig.Symbol.OID.
type SymbolConfigCompat SymbolConfig

// SymbolConfig holds info for a single symbol/oid
type SymbolConfig struct {
	OID  string `yaml:"OID,omitempty" json:"OID,omitempty"`
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Documentation-only: This field exists solely so hand-written profiles can document where they found a symbol.
	MIB string `yaml:"MIB,omitempty" json:"-"`

	ExtractValue         string         `yaml:"extract_value,omitempty" json:"extract_value,omitempty"`
	ExtractValueCompiled *regexp.Regexp `yaml:"-" json:"-"`

	MatchPattern         string         `yaml:"match_pattern,omitempty" json:"match_pattern,omitempty"`
	MatchValue           string         `yaml:"match_value,omitempty" json:"match_value,omitempty"`
	MatchPatternCompiled *regexp.Regexp `yaml:"-" json:"-"`

	ScaleFactor      float64 `yaml:"scale_factor,omitempty" json:"scale_factor,omitempty"`
	Format           string  `yaml:"format,omitempty" json:"format,omitempty"`
	ConstantValueOne bool    `yaml:"constant_value_one,omitempty" json:"constant_value_one,omitempty"`

	// `metric_type` is used for force the metric type
	//   When empty, by default, the metric type is derived from SNMP OID value type.
	//   Valid `metric_type` types: `gauge`, `rate`, `monotonic_count`, `monotonic_count_and_rate`
	//   Deprecated types: `counter` (use `rate` instead), percent (use `scale_factor` instead)
	MetricType ProfileMetricType `yaml:"metric_type,omitempty" json:"metric_type,omitempty"`
}

// MetricTagConfig holds metric tag info
type MetricTagConfig struct {
	Tag string `yaml:"tag" json:"tag"`

	// Table config
	Index uint `yaml:"index,omitempty" json:"index,omitempty"`

	// Documentation-only: These fields exist solely so hand-written yaml profiles can document where they found a symbol.
	MIB   string `yaml:"MIB,omitempty" json:"-"`
	Table string `yaml:"table,omitempty" json:"-"`

	// DEPRECATED: Column and OID are replaced by the Symbol field
	Column SymbolConfig `yaml:"column,omitempty" json:"-"`
	OID    string       `yaml:"OID,omitempty" json:"-"` // DEPRECATED replaced by Symbol field

	// SymbolConfigCompat will also parse a plain string, but this behavior is deprecated.
	Symbol SymbolConfigCompat `yaml:"symbol,omitempty" json:"symbol,omitempty"`

	IndexTransform []MetricIndexTransform `yaml:"index_transform,omitempty" json:"index_transform,omitempty"`

	Mapping ListMap[string] `yaml:"mapping,omitempty" json:"mapping,omitempty"`

	// Regex
	// Match/Tags are not exposed as json (UI) since ExtractValue can be used instead
	Match   string            `yaml:"match,omitempty" json:"-"`
	Tags    map[string]string `yaml:"tags,omitempty" json:"-"`
	Pattern *regexp.Regexp    `yaml:"-" json:"-"`

	SymbolTag string `yaml:"-" json:"-"`
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
	Placement    uint   `yaml:"placement,omitempty" json:"placement,omitempty"`
	MetricSuffix string `yaml:"metric_suffix,omitempty" json:"metric_suffix,omitempty"`
}

// MetricsConfig holds configs for a metric
type MetricsConfig struct {
	// MIB the MIB used for this metric
	MIB string `yaml:"MIB,omitempty" json:"MIB,omitempty"`

	// Table the table OID
	Table SymbolConfig `yaml:"table,omitempty" json:"table,omitempty"`

	// Symbol configs
	Symbol SymbolConfig `yaml:"symbol,omitempty" json:"symbol,omitempty"`

	// DEPRECATED: These have been moved into Symbol and/or Symbols
	OID        string            `yaml:"OID,omitempty" json:"-" jsonschema:"-"`
	Name       string            `yaml:"name,omitempty" json:"-" jsonschema:"-"`
	ForcedType ProfileMetricType `yaml:"forced_type,omitempty" json:"-"` // renamed to MetricType
	MetricType ProfileMetricType `yaml:"metric_type,omitempty" json:"-"` // moved into Symbol/Symbols

	// Table configs
	Symbols []SymbolConfig `yaml:"symbols,omitempty" json:"symbols,omitempty"`

	// `static_tags` is not exposed as json at the moment since we need to evaluate if we want to expose it via UI
	StaticTags []string            `yaml:"static_tags,omitempty" json:"-"`
	MetricTags MetricTagConfigList `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`

	// `options` is not exposed as json at the moment since we need to evaluate if we want to expose it via UI
	Options MetricsConfigOption `yaml:"options,omitempty" json:"-"`
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
