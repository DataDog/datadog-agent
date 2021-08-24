package snmp

import (
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ResultValueStore store OID values
type ResultValueStore struct {
	// TODO: make fields private?
	ScalarValues scalarResultValuesType
	ColumnValues columnResultValuesType
}

// getScalarValue look for oid in ResultValueStore and returns the value and boolean
// weather valid value has been found
func (v *ResultValueStore) getScalarValue(oid string) (ResultValue, error) {
	value, ok := v.ScalarValues[oid]
	if !ok {
		return ResultValue{}, fmt.Errorf("value for Scalar OID `%s` not found in results", oid)
	}
	return value, nil
}

// getColumnValues look for oid in ResultValueStore and returns a map[<fullIndex>]ResultValue
// where `fullIndex` refer to the entire index part of the instance OID.
// For example if the row oid (instance oid) is `1.3.6.1.4.1.1.2.3.10.11.12`,
// the column oid is `1.3.6.1.4.1.1.2.3`, the fullIndex is `10.11.12`.
func (v *ResultValueStore) getColumnValues(oid string) (map[string]ResultValue, error) {
	values, ok := v.ColumnValues[oid]
	if !ok {
		return nil, fmt.Errorf("value for Column OID `%s` not found in results", oid)
	}
	retValues := make(map[string]ResultValue, len(values))
	for index, value := range values {
		retValues[index] = value
	}

	return retValues, nil
}

// getColumnValue look for oid in ResultValueStore and returns a ResultValue
func (v *ResultValueStore) getColumnValue(oid string, index string) (ResultValue, error) {
	values, ok := v.ColumnValues[oid]
	if !ok {
		return ResultValue{}, fmt.Errorf("value for Column OID `%s` not found in results", oid)
	}
	value, ok := values[index]
	if !ok {
		return ResultValue{}, fmt.Errorf("value for Column OID `%s` and index `%s` not found in results", oid, index)
	}
	return value, nil
}

func (v *ResultValueStore) getScalarValueAsString(oid string) string {
	value, err := v.getScalarValue(oid)
	if err != nil {
		log.Tracef("failed to get value for OID %s: %s", oid, err)
		return ""
	}
	str, err := value.toString()
	if err != nil {
		log.Tracef("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return ""
	}
	return str
}

// getColumnValueAsString look for oid/index in ResultValueStore and returns a string
func (v *ResultValueStore) getColumnValueAsString(oid string, index string) string {
	value, err := v.getColumnValue(oid, index)
	if err != nil {
		log.Tracef("failed to get value for OID %s with index %s: %s", oid, index, err)
		return ""
	}
	str, err := value.toString()
	if err != nil {
		log.Tracef("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return ""
	}
	return str
}

// getColumnValueAsFloat look for oid/index in ResultValueStore and returns a float64
func (v *ResultValueStore) getColumnValueAsFloat(oid string, index string) float64 {
	value, err := v.getColumnValue(oid, index)
	if err != nil {
		log.Tracef("failed to get value for OID %s with index %s: %s", oid, index, err)
		return 0
	}
	floatValue, err := value.toFloat64()
	if err != nil {
		log.Tracef("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return 0
	}
	return floatValue
}

func (v *ResultValueStore) getColumnIndexes(columnOid string) ([]string, error) {
	indexesMap := make(map[string]struct{})
	metricValues, err := v.getColumnValues(columnOid)
	if err != nil {
		return nil, fmt.Errorf("error getting column value oid=%s: %s", columnOid, err)
	}
	for fullIndex := range metricValues {
		indexesMap[fullIndex] = struct{}{}
	}

	var indexes []string
	for index := range indexesMap {
		indexes = append(indexes, index)
	}

	sort.Strings(indexes) // sort indexes for better consistency
	return indexes, nil
}
