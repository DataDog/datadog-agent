package snmp

import (
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type resultValueStore struct {
	scalarValues scalarResultValuesType
	columnValues columnResultValuesType
}

// getScalarValue look for oid in resultValueStore and returns the value and boolean
// weather valid value has been found
func (v *resultValueStore) getScalarValue(oid string) (snmpValueType, error) {
	value, ok := v.scalarValues[oid]
	if !ok {
		return snmpValueType{}, fmt.Errorf("value for Scalar OID `%s` not found in results", oid)
	}
	return value, nil
}

// getColumnValues look for oid in resultValueStore and returns a map[<fullIndex>]snmpValueType
// where `fullIndex` refer to the entire index part of the instance OID.
// For example if the row oid (instance oid) is `1.3.6.1.4.1.1.2.3.10.11.12`,
// the column oid is `1.3.6.1.4.1.1.2.3`, the fullIndex is `10.11.12`.
func (v *resultValueStore) getColumnValues(oid string) (map[string]snmpValueType, error) {
	values, ok := v.columnValues[oid]
	if !ok {
		return nil, fmt.Errorf("value for Column OID `%s` not found in results", oid)
	}
	retValues := make(map[string]snmpValueType, len(values))
	for index, value := range values {
		retValues[index] = value
	}

	return retValues, nil
}

// getColumnValue look for oid in resultValueStore and returns a snmpValueType
func (v *resultValueStore) getColumnValue(oid string, index string) (snmpValueType, error) {
	values, ok := v.columnValues[oid]
	if !ok {
		return snmpValueType{}, fmt.Errorf("value for Column OID `%s` not found in results", oid)
	}
	value, ok := values[index]
	if !ok {
		return snmpValueType{}, fmt.Errorf("value for Column OID `%s` and index `%s` not found in results", oid, index)
	}
	return value, nil
}

func (v *resultValueStore) getScalarValueAsString(oid string) string {
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

// getColumnValueAsString look for oid/index in resultValueStore and returns a string
func (v *resultValueStore) getColumnValueAsString(oid string, index string) string {
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

// getColumnValueAsFloat look for oid/index in resultValueStore and returns a float64
func (v *resultValueStore) getColumnValueAsFloat(oid string, index string) float64 {
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

func (v *resultValueStore) getColumnIndexes(columnOid string) ([]string, error) {
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
