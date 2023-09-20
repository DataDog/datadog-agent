// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

const (
	ifTablePrefix  = "1.3.6.1.2.1.2.2."
	ifXTablePrefix = "1.3.6.1.2.1.31.1.1."
)

func getScalarValueFromSymbol(values *valuestore.ResultValueStore, symbol profiledefinition.SymbolConfig) (valuestore.ResultValue, error) {
	value, err := values.GetScalarValue(symbol.OID)
	if err != nil {
		return valuestore.ResultValue{}, err
	}
	return processValueUsingSymbolConfig(value, symbol)
}

func getColumnValueFromSymbol(values *valuestore.ResultValueStore, symbol profiledefinition.SymbolConfig) (map[string]valuestore.ResultValue, error) {
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

func processValueUsingSymbolConfig(value valuestore.ResultValue, symbol profiledefinition.SymbolConfig) (valuestore.ResultValue, error) {
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
			log.Debugf("error converting value to string (value=%v): %v", value, err)
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
	if symbol.Format != "" {
		var err error
		value, err = formatValue(value, symbol.Format)
		if err != nil {
			return valuestore.ResultValue{}, err
		}
	}
	return value, nil
}

// getTagsFromMetricTagConfigList retrieve tags using the metric config and values
func getTagsFromMetricTagConfigList(mtcl profiledefinition.MetricTagConfigList, fullIndex string, values *valuestore.ResultValueStore) []string {
	var rowTags []string
	indexes := strings.Split(fullIndex, ".")
	for _, metricTag := range mtcl {
		// get tag using `index` field
		if metricTag.Index > 0 {
			index := metricTag.Index - 1 // `index` metric config is 1-based
			if index >= uint(len(indexes)) {
				log.Debugf("error getting tags. index `%d` not found in indexes `%v`", metricTag.Index, indexes)
				continue
			}
			tagValue, err := checkconfig.GetMappedValue(indexes[index], metricTag.Mapping)
			if err != nil {
				log.Debugf("error getting tags. mapping for `%s` does not exist. mapping=`%v`, indexes=`%v`", indexes[index], metricTag.Mapping, indexes)
				continue
			}
			rowTags = append(rowTags, metricTag.Tag+":"+tagValue)
		}
		// get tag using another column value
		if metricTag.Column.OID != "" {
			// TODO: Support extract value see II-635
			columnValues, err := getColumnValueFromSymbol(values, metricTag.Column)
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
			rowTags = append(rowTags, checkconfig.BuildMetricTagsFromValue(&metricTag, strValue)...)
		}
	}
	return rowTags
}

// transformIndex change a source index into a new index using a list of transform rules.
// A transform rule has start/end fields, it is used to extract a subset of the source index.
func transformIndex(indexes []string, transformRules []profiledefinition.MetricIndexTransform) []string {
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

func netmaskToPrefixlen(netmask string) int {
	stringMask := net.IPMask(net.ParseIP(netmask).To4())
	length, _ := stringMask.Size()
	return length
}

// getInterfaceConfig retrieves snmpintegration.InterfaceConfig by index and tags
func getInterfaceConfig(interfaceConfigs []snmpintegration.InterfaceConfig, index string, tags []string) (snmpintegration.InterfaceConfig, error) {
	var ifName string
	for _, tag := range tags {
		tagElems := strings.SplitN(tag, ":", 2)
		if len(tagElems) == 2 && tagElems[0] == "interface" {
			ifName = tagElems[1]
			break
		}
	}
	for _, ifConfig := range interfaceConfigs {
		if (ifConfig.MatchField == "name" && ifConfig.MatchValue == ifName) ||
			(ifConfig.MatchField == "index" && ifConfig.MatchValue == index) {
			return ifConfig, nil
		}
	}
	return snmpintegration.InterfaceConfig{}, fmt.Errorf("no matching interface found for index=%s, tags=%s", index, tags)
}

// getConstantMetricValues retrieve all metric tags indexes and set their value as 1
func getConstantMetricValues(mtcl profiledefinition.MetricTagConfigList, values *valuestore.ResultValueStore) map[string]valuestore.ResultValue {
	constantValues := make(map[string]valuestore.ResultValue)
	for _, metricTag := range mtcl {
		if len(metricTag.IndexTransform) > 0 {
			// If index transform is set, indexes are from another table, we don't want to collect them
			continue
		}
		if metricTag.Column.OID != "" {
			columnValues, err := getColumnValueFromSymbol(values, metricTag.Column)
			if err != nil {
				log.Debugf("error getting column value: %v", err)
				continue
			}
			for index := range columnValues {
				if _, ok := constantValues[index]; ok {
					continue
				}
				constantValues[index] = valuestore.ResultValue{
					Value: float64(1),
				}
			}
		}
	}
	return constantValues
}

// isInterfaceTableMetric takes in an OID and returns
// true if the prefix matches ifTable or ifXTable from
// the IF-MIB
func isInterfaceTableMetric(oid string) bool {
	oid = strings.TrimPrefix(oid, ".")
	return strings.HasPrefix(oid, ifTablePrefix) || strings.HasPrefix(oid, ifXTablePrefix)
}
