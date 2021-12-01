package report

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getScalarValueFromSymbol(values *valuestore.ResultValueStore, symbol checkconfig.SymbolConfig) (valuestore.ResultValue, error) {
	value, err := values.GetScalarValue(symbol.OID)
	if err != nil {
		return valuestore.ResultValue{}, err
	}
	if symbol.ExtractValueCompiled != nil {
		extractedValue, err := value.ExtractStringValue(symbol.ExtractValueCompiled)
		if err != nil {
			log.Debugf("error extracting value from `%v` with pattern `%v`: %v", value, symbol.ExtractValueCompiled, err)
			return valuestore.ResultValue{}, err
		}
		value = extractedValue
	}
	return value, nil
}

func getColumnValueFromSymbol(values *valuestore.ResultValueStore, symbol checkconfig.SymbolConfig) (map[string]valuestore.ResultValue, error) {
	columnValues, err := values.GetColumnValues(symbol.OID)
	newValues := make(map[string]valuestore.ResultValue, len(columnValues))
	if err != nil {
		return nil, err
	}
	for index, value := range columnValues {
		if symbol.ExtractValueCompiled != nil {
			extractedValue, err := value.ExtractStringValue(symbol.ExtractValueCompiled)
			if err != nil {
				log.Debugf("error extracting value from `%v` with pattern `%v`: %v", value, symbol.ExtractValueCompiled, err)
				continue
			}
			value = extractedValue
		}
		newValues[index] = value
	}
	return newValues, nil
}
