package report

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getScalarValueFromSymbol(values *valuestore.ResultValueStore, symbol checkconfig.SymbolConfig) (valuestore.ResultValue, error) {
	value, err := values.GetScalarValue(symbol.OID)
	if err != nil {
		return valuestore.ResultValue{}, err
	}
	return processValue(symbol, value)
}

func getColumnValueFromSymbol(values *valuestore.ResultValueStore, symbol checkconfig.SymbolConfig) (map[string]valuestore.ResultValue, error) {
	columnValues, err := values.GetColumnValues(symbol.OID)
	newValues := make(map[string]valuestore.ResultValue, len(columnValues))
	if err != nil {
		return nil, err
	}
	for index, value := range columnValues {
		// TODO: TEST ME
		newValue, err := processValue(symbol, value)
		if err != nil {
			continue
		}
		newValues[index] = newValue
	}
	return newValues, nil
}

func processValue(symbol checkconfig.SymbolConfig, value valuestore.ResultValue) (valuestore.ResultValue, error) {
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
				return valuestore.ResultValue{}, fmt.Errorf("pattern `%v` failed to match `%v` with template `%s`", symbol.MatchPattern, strValue, symbol.MatchValue)
			}
			value = valuestore.ResultValue{Value: replacedVal}
		} else {
			return valuestore.ResultValue{}, fmt.Errorf("match pattern `%v` does not match string `%s`", symbol.MatchPattern, strValue)
		}
	}
	return value, nil
}
