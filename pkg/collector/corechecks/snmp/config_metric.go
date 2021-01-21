package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"regexp"
	"strings"
)

type symbolConfig struct {
	OID  string `yaml:"OID"`
	Name string `yaml:"name"`
}

type metricTagConfig struct {
	Tag string `yaml:"tag"`

	// Table config
	Index  uint         `yaml:"index"`
	Column symbolConfig `yaml:"column"`

	// Symbol config
	OID  string `yaml:"OID"`
	Name string `yaml:"symbol"`

	IndexTransform []metricIndexTransform `yaml:"index_transform"`

	Mapping map[string]string `yaml:"mapping"`

	// Regex
	Match string            `yaml:"match"`
	Tags  map[string]string `yaml:"tags"`

	symbolTag string
}

type metricTagConfigList []metricTagConfig

type metricIndexTransform struct {
	Start uint `yaml:"start"`
	End   uint `yaml:"end"`
}

type metricsConfigOption struct {
	Placement    uint   `yaml:"placement"`
	MetricSuffix string `yaml:"metric_suffix"`
}

type metricsConfig struct {
	// Symbol configs
	Symbol symbolConfig `yaml:"symbol"`

	// Legacy Symbol configs syntax
	OID  string `yaml:"OID"`
	Name string `yaml:"name"`

	// Table configs
	Table   symbolConfig   `yaml:"table"`
	Symbols []symbolConfig `yaml:"symbols"`

	MetricTags metricTagConfigList `yaml:"metric_tags"`

	ForcedType string              `yaml:"forced_type"`
	Options    metricsConfigOption `yaml:"options"`

	// TODO: [VALIDATION] Validate Symbol and Table are not both used
}

// getTags retrieve tags using the metric config and values
func (m *metricsConfig) getTags(fullIndex string, values *resultValueStore) []string {
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
			//tagValueOid := metricTag.Column.OID + "." + fullIndex
			stringValues, err := values.getColumnValues(metricTag.Column.OID)
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

			tagValue, ok := stringValues[newFullIndex]
			if !ok {
				log.Debugf("index not found for column value: tag=%v, index=%v", metricTag.Tag, newFullIndex)
			} else {
				rowTags = append(rowTags, metricTag.getTags(tagValue.toString())...)
			}
		}
	}
	return rowTags
}

func (m *metricsConfig) getSymbolTags() []string {
	var symbolTags []string
	for _, metricTag := range m.MetricTags {
		symbolTags = append(symbolTags, metricTag.symbolTag)
	}
	return symbolTags
}

func (mtc metricTagConfig) getTags(value string) []string {
	var tags []string
	if mtc.Tag != "" {
		tags = append(tags, mtc.Tag+":"+value)
	} else if mtc.Match != "" {
		re, err := regexp.Compile(mtc.Match) // TODO: [VALIDATION] may fail, compile in config validation
		if err != nil {
			log.Warnf("failed to compile `%v` from metric tag config `%v`", mtc.Match, mtc)
			return tags
		}
		if re.MatchString(value) {
			for key, val := range mtc.Tags {
				normalizedVal := normalizeRegexReplaceValue(val)
				replacedVal := re.ReplaceAllString(value, normalizedVal)
				tags = append(tags, key+":"+replacedVal)
			}
		}
	}
	// TODO: [VALIDATION] Handle error case in config validation
	return tags
}

// normalizeRegexReplaceValue normalize regex value to keep compatibility with Python
// Converts \1 into $1, \2 into $2, etc
func normalizeRegexReplaceValue(val string) string {
	re := regexp.MustCompile("\\\\(\\d+)")
	return re.ReplaceAllString(val, "$$$1")
}

// transformIndex change a source index into a new index using a list of transform rules.
// A transform rule has start/end fields, it is used to extract a subset of the source index.
func transformIndex(indexes []string, transformRules []metricIndexTransform) []string {
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
func normalizeMetrics(metrics []metricsConfig) {
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
