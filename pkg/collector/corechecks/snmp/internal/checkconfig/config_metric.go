// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ProfileMetricType string

const (
	ProfileMetricTypeGauge                 ProfileMetricType = "gauge"
	ProfileMetricTypeMonotonicCount        ProfileMetricType = "monotonic_count"
	ProfileMetricTypeMonotonicCountAndRate ProfileMetricType = "monotonic_count_and_rate"
	ProfileMetricTypeRate                  ProfileMetricType = "rate"
	ProfileMetricTypeFlagStream            ProfileMetricType = "flag_stream"

	// ProfileMetricTypeCounter is DEPRECATED
	// `counter` is deprecated in favour of `rate`
	ProfileMetricTypeCounter ProfileMetricType = "counter"

	// ProfileMetricTypePercent is DEPRECATED
	// `percent` is deprecated in favour of `scale_factor`
	ProfileMetricTypePercent ProfileMetricType = "percent"
)

// SymbolConfig holds info for a single symbol/oid
type SymbolConfig struct {
	OID  string `yaml:"OID"`
	Name string `yaml:"name"`

	ExtractValue         string `yaml:"extract_value"`
	ExtractValueCompiled *regexp.Regexp

	MatchPattern         string `yaml:"match_pattern"`
	MatchValue           string `yaml:"match_value"`
	MatchPatternCompiled *regexp.Regexp

	ScaleFactor      float64 `yaml:"scale_factor"`
	Format           string  `yaml:"format"`
	ConstantValueOne bool    `yaml:"constant_value_one"`

	// `metric_type` is used for force the metric type
	//   When empty, by default, the metric type is derived from SNMP OID value type.
	//   Valid `metric_type` types: `gauge`, `rate`, `monotonic_count`, `monotonic_count_and_rate`
	//   Deprecated types: `counter` (use `rate` instead), percent (use `scale_factor` instead)
	MetricType ProfileMetricType `yaml:"metric_type"`
}

// MetricTagConfig holds metric tag info
type MetricTagConfig struct {
	Tag string `yaml:"tag"`

	// Table config
	Index uint `yaml:"index"`

	// TODO: refactor to rename to `symbol` instead (keep backward compat with `column`)
	Column SymbolConfig `yaml:"column"`

	// Symbol config
	OID  string `yaml:"OID"`
	Name string `yaml:"symbol"`

	IndexTransform []MetricIndexTransform `yaml:"index_transform"`

	Mapping map[string]string `yaml:"mapping"`

	// Regex
	Match string            `yaml:"match"`
	Tags  map[string]string `yaml:"tags"`

	symbolTag string
	pattern   *regexp.Regexp
}

// MetricTagConfigList holds configs for a list of metric tags
type MetricTagConfigList []MetricTagConfig

// MetricIndexTransform holds configs for metric index transform
type MetricIndexTransform struct {
	Start uint `yaml:"start"`
	End   uint `yaml:"end"`
}

// MetricsConfigOption holds config for metrics options
type MetricsConfigOption struct {
	Placement    uint   `yaml:"placement"`
	MetricSuffix string `yaml:"metric_suffix"`
}

// MetricsConfig holds configs for a metric
type MetricsConfig struct {
	// Symbol configs
	Symbol SymbolConfig `yaml:"symbol"`

	// Legacy Symbol configs syntax
	OID  string `yaml:"OID"`
	Name string `yaml:"name"`

	// Table configs
	Symbols []SymbolConfig `yaml:"symbols"`

	StaticTags []string            `yaml:"static_tags"`
	MetricTags MetricTagConfigList `yaml:"metric_tags"`

	ForcedType ProfileMetricType `yaml:"forced_type"` // deprecated in favour of metric_type
	MetricType ProfileMetricType `yaml:"metric_type"`

	Options MetricsConfigOption `yaml:"options"`
}

// GetSymbolTags returns symbol tags
func (m *MetricsConfig) GetSymbolTags() []string {
	var symbolTags []string
	for _, metricTag := range m.MetricTags {
		symbolTags = append(symbolTags, metricTag.symbolTag)
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

// GetTags returns tags based on MetricTagConfig and a value
func (mtc *MetricTagConfig) GetTags(value string) []string {
	var tags []string
	if mtc.Tag != "" {
		if len(mtc.Mapping) > 0 {
			mappedValue, err := GetMappedValue(value, mtc.Mapping)
			if err != nil {
				log.Debugf("error getting tags. mapping for `%s` does not exist. mapping=`%v`", value, mtc.Mapping)
			} else {
				tags = append(tags, mtc.Tag+":"+mappedValue)
			}
		} else {
			tags = append(tags, mtc.Tag+":"+value)
		}
	} else if mtc.Match != "" {
		if mtc.pattern == nil {
			log.Warnf("match pattern must be present: match=%s", mtc.Match)
			return tags
		}
		if mtc.pattern.MatchString(value) {
			for key, val := range mtc.Tags {
				normalizedTemplate := normalizeRegexReplaceValue(val)
				replacedVal := RegexReplaceValue(value, mtc.pattern, normalizedTemplate)
				if replacedVal == "" {
					log.Debugf("pattern `%v` failed to match `%v` with template `%v`", mtc.pattern, value, normalizedTemplate)
					continue
				}
				tags = append(tags, key+":"+replacedVal)
			}
		}
	}
	return tags
}

// RegexReplaceValue replaces a value using a regex and template
func RegexReplaceValue(value string, pattern *regexp.Regexp, normalizedTemplate string) string {
	result := []byte{}
	for _, submatches := range pattern.FindAllStringSubmatchIndex(value, 1) {
		result = pattern.ExpandString(result, normalizedTemplate, value, submatches)
	}
	return string(result)
}

// normalizeRegexReplaceValue normalize regex value to keep compatibility with Python
// Converts \1 into $1, \2 into $2, etc
func normalizeRegexReplaceValue(val string) string {
	re := regexp.MustCompile(`\\(\d+)`)
	return re.ReplaceAllString(val, "$$$1")
}

// normalizeMetrics converts legacy syntax to new syntax
// 1/ converts old symbol syntax to new symbol syntax
// metric.Name and metric.OID info are moved to metric.Symbol.Name and metric.Symbol.OID
func normalizeMetrics(metrics []MetricsConfig) {
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

// GetMappedValue retrieves mapped value from a given mapping.
// If mapping is empty, it will return the index.
func GetMappedValue(index string, mapping map[string]string) (string, error) {
	if len(mapping) > 0 {
		mappedValue, ok := mapping[index]
		if !ok {
			return "", fmt.Errorf("mapping for `%s` does not exist. mapping=`%v`", index, mapping)
		}
		return mappedValue, nil
	}
	return index, nil
}
