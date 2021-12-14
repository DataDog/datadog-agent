package report

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strings"
)

func getScalarValueFromSymbol(values *valuestore.ResultValueStore, symbol checkconfig.SymbolConfig) (valuestore.ResultValue, error) {
	value, err := values.GetScalarValue(symbol.OID)
	if err != nil {
		return valuestore.ResultValue{}, err
	}
	return processValueUsingSymbolConfig(value, symbol)
}

func getColumnValueFromSymbol(values *valuestore.ResultValueStore, symbol checkconfig.SymbolConfig) (map[string]valuestore.ResultValue, error) {
	columnValues, err := values.GetColumnValues(symbol.OID)
	newValues := make(map[string]valuestore.ResultValue, len(columnValues))
	if err != nil {
		return nil, err
	}
	for index, value := range columnValues {
		newValue, err := processValueUsingSymbolConfig(value, symbol)
		if err != nil {
			continue
		}
		newValues[index] = newValue
	}
	return newValues, nil
}

func getColumnValueFromSymbolOrEmpty(oid string, values *valuestore.ResultValueStore) map[string]valuestore.ResultValue {
	if oid == "" {
		return nil
	}
	columnValues, err := getColumnValueFromSymbol(values, checkconfig.SymbolConfig{OID: oid})
	if err != nil {
		log.Debugf("error getting index values: %v", err)
		return nil
	}
	return columnValues
}

func processValueUsingSymbolConfig(value valuestore.ResultValue, symbol checkconfig.SymbolConfig) (valuestore.ResultValue, error) {
	if symbol.ExtractValueCompiled != nil {
		extractedValue, err := value.ExtractStringValue(symbol.ExtractValueCompiled)
		if err != nil {
			log.Debugf("error extracting value from `%v` with pattern `%v`: %v", value, symbol.ExtractValueCompiled, err)
			return valuestore.ResultValue{}, err
		}
		value = extractedValue
	}
	if symbol.MatchPatternCompiled != nil {
		strValue, err := value.ToString()
		if err != nil {
			log.Debugf("error converting value to string (value=%v):", value, err)
			return valuestore.ResultValue{}, err
		}

		if symbol.MatchPatternCompiled.MatchString(strValue) {
			replacedVal := checkconfig.RegexReplaceValue(strValue, symbol.MatchPatternCompiled, symbol.MatchValue)
			if replacedVal == "" {
				return valuestore.ResultValue{}, fmt.Errorf("the pattern `%v` matched value `%v`, but template `%s` is not compatible", symbol.MatchPattern, strValue, symbol.MatchValue)
			}
			value = valuestore.ResultValue{Value: replacedVal}
		} else {
			return valuestore.ResultValue{}, fmt.Errorf("match pattern `%v` does not match string `%s`", symbol.MatchPattern, strValue)
		}
	}
	return value, nil
}

// getTagsUsingMetricConfig retrieve tags using the metric config and values
func getTagsUsingMetricConfig(tagConfigs checkconfig.MetricTagConfigList, fullIndex string, values *valuestore.ResultValueStore) []string {
	var rowTags []string
	for _, metricTag := range tagConfigs {
		indexTag, err := getIndexTags(fullIndex, metricTag)
		if err != nil {
			log.Debugf("get index tag error: %v", err)
			continue
		}
		if indexTag != "" {
			rowTags = append(rowTags, indexTag)
		}

		// get tag using column value
		if metricTag.Column.OID != "" {
			// TODO: Support extract value see II-635
			columnValues, err := values.GetColumnValues(metricTag.Column.OID)
			if err != nil {
				log.Debugf("error getting column value: %v", err)
				continue
			}

			newIndex := fullIndex
			newIndex, err = newIndexUsingIndexFromOidValue(newIndex, metricTag.Column.IndexFromOidValue.OID, values)
			if err != nil {
				log.Debugf(err.Error())
				continue
			}
			newIndex = newIndexUsingTransformIndex(newIndex, metricTag.IndexTransform)

			tagValue, ok := columnValues[newIndex]
			if !ok {
				log.Debugf("index not found for column value: tag=%v, index=%v", metricTag.Tag, newIndex)
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

func newIndexUsingIndexFromOidValue(fullIndex string, indexFromOidValueOid string, values *valuestore.ResultValueStore) (string, error) {
	if indexFromOidValueOid == "" {
		return fullIndex, nil
	}
	indexValues := getColumnValueFromSymbolOrEmpty(indexFromOidValueOid, values)
	for index, indexVal := range indexValues {
		strVal, err := indexVal.ToString()
		if err != nil {
			log.Debugf("error converting index value: %v", err)
			continue
		}
		if strVal == fullIndex {
			return index, nil
		}
	}
	return "", fmt.Errorf("failed to get new index using index_from_oid_value `%s`", indexFromOidValueOid)
}

func newIndexUsingTransformIndex(fullIndex string, indexTransforms []checkconfig.MetricIndexTransform) string {
	indexes := strings.Split(fullIndex, ".")
	var newIndexes []string
	if len(indexTransforms) > 0 {
		newIndexes = transformIndex(indexes, indexTransforms)
	} else {
		newIndexes = indexes
	}
	newFullIndex := strings.Join(newIndexes, ".")
	return newFullIndex
}

// getIndexTags gets index tag using `index` field
func getIndexTags(fullIndex string, metricTag checkconfig.MetricTagConfig) (string, error) {
	indexes := strings.Split(fullIndex, ".")

	if metricTag.Index == 0 {
		return "", nil
	}
	index := metricTag.Index - 1 // `index` metric config is 1-based
	if index >= uint(len(indexes)) {
		return "", fmt.Errorf("error getting tags. index `%d` not found in indexes `%v`", metricTag.Index, indexes)
	}
	var tagValue string
	if len(metricTag.Mapping) > 0 {
		mappedValue, ok := metricTag.Mapping[indexes[index]]
		if !ok {
			return "", fmt.Errorf("error getting tags. mapping for `%s` does not exist. mapping=`%v`, indexes=`%v`", indexes[index], metricTag.Mapping, indexes)
		}
		tagValue = mappedValue
	} else {
		tagValue = indexes[index]
	}
	indexTag := metricTag.Tag + ":" + tagValue
	return indexTag, nil
}

// transformIndex change a source index into a new index using a list of transform rules.
// A transform rule has start/end fields, it is used to extract a subset of the source index.
func transformIndex(indexes []string, transformRules []checkconfig.MetricIndexTransform) []string {
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
