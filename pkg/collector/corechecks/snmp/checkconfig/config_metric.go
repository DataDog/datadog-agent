package checkconfig

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

// SymbolConfig holds info for a single symbol/oid
type SymbolConfig struct {
	OID          string `yaml:"OID"`
	Name         string `yaml:"name"`
	ExtractValue string `yaml:"extract_value"`

	ExtractValuePattern *regexp.Regexp
}

// MetricTagConfig holds metric tag info
type MetricTagConfig struct {
	Tag string `yaml:"tag"`

	// Table config
	Index  uint         `yaml:"index"`
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

	MetricTags MetricTagConfigList `yaml:"metric_tags"`

	ForcedType string              `yaml:"forced_type"`
	Options    MetricsConfigOption `yaml:"options"`
}

// GetTags retrieve tags using the metric config and values
func (m *MetricsConfig) GetTags(fullIndex string, values *valuestore.ResultValueStore) []string {
	var rowTags []string
	indexes := strings.Split(fullIndex, ".")
	for _, metricTag := range m.MetricTags {
		// get tag using `index` field
		if metricTag.Index > 0 {
			index := metricTag.Index - 1 // `index` metric config is 1-based
			if index >= uint(len(indexes)) {
				log.Debugf("error getting tags. index `%d` not found in indexes `%v`", metricTag.Index, indexes)
				continue
			}
			var tagValue string
			if len(metricTag.Mapping) > 0 {
				mappedValue, ok := metricTag.Mapping[indexes[index]]
				if !ok {
					log.Debugf("error getting tags. mapping for `%s` does not exist. mapping=`%v`, indexes=`%v`", indexes[index], metricTag.Mapping, indexes)
					continue
				}
				tagValue = mappedValue
			} else {
				tagValue = indexes[index]
			}
			rowTags = append(rowTags, metricTag.Tag+":"+tagValue)
		}
		// get tag using another column value
		if metricTag.Column.OID != "" {
			columnValues, err := values.GetColumnValues(metricTag.Column.OID)
			if err != nil {
				log.Debugf("error getting column value: %v", err)
				continue
			}

			var newIndexes []string
			if len(metricTag.IndexTransform) > 0 {
				newIndexes = transformIndex(indexes, metricTag.IndexTransform)
			} else {
				newIndexes = indexes
			}
			newFullIndex := strings.Join(newIndexes, ".")

			tagValue, ok := columnValues[newFullIndex]
			if !ok {
				log.Debugf("index not found for column value: tag=%v, index=%v", metricTag.Tag, newFullIndex)
				continue
			}
			strValue, err := tagValue.ToString()
			if err != nil {
				log.Debugf("error converting tagValue (%#v) to string : %v", tagValue, err)
				continue
			}
			rowTags = append(rowTags, metricTag.GetTags(strValue)...)
		}
	}
	return rowTags
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
		tags = append(tags, mtc.Tag+":"+value)
	} else if mtc.Match != "" {
		if mtc.pattern == nil {
			log.Warnf("match pattern must be present: match=%s", mtc.Match)
			return tags
		}
		if mtc.pattern.MatchString(value) {
			for key, val := range mtc.Tags {
				normalizedTemplate := normalizeRegexReplaceValue(val)
				replacedVal := regexReplaceValue(value, mtc.pattern, normalizedTemplate)
				if replacedVal == "" {
					log.Debugf("pattern `%v` failed to match `%v` with template `%v`", value, normalizedTemplate)
					continue
				}
				tags = append(tags, key+":"+replacedVal)
			}
		}
	}
	return tags
}

func regexReplaceValue(value string, pattern *regexp.Regexp, normalizedTemplate string) string {
	result := []byte{}
	for _, submatches := range pattern.FindAllStringSubmatchIndex(value, 1) {
		result = pattern.ExpandString(result, normalizedTemplate, value, submatches)
	}
	return string(result)
}

// normalizeRegexReplaceValue normalize regex value to keep compatibility with Python
// Converts \1 into $1, \2 into $2, etc
func normalizeRegexReplaceValue(val string) string {
	re := regexp.MustCompile("\\\\(\\d+)")
	return re.ReplaceAllString(val, "$$$1")
}

// transformIndex change a source index into a new index using a list of transform rules.
// A transform rule has start/end fields, it is used to extract a subset of the source index.
func transformIndex(indexes []string, transformRules []MetricIndexTransform) []string {
	var newIndex []string

	for _, rule := range transformRules {
		start := rule.Start
		end := rule.End + 1
		if end > uint(len(indexes)) {
			return nil
		}
		newIndex = append(newIndex, indexes[start:end]...)
	}
	return newIndex
}

// normalizeMetrics converts legacy syntax to new syntax
// 1/ converts old symbol syntax to new symbol syntax
//    metric.Name and metric.OID info are moved to metric.Symbol.Name and metric.Symbol.OID
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
