package snmp

import (
	"fmt"
)

type resultValueStore struct {
	scalarValues scalarResultValuesType
	columnValues columnResultValuesType
}

// getScalarValues look for oid in resultValueStore and returns the value and boolean
// weather valid value has been found
func (v *resultValueStore) getScalarValues(oid string) (snmpValueType, error) {
	value, ok := v.scalarValues[oid]
	if !ok {
		return snmpValueType{}, fmt.Errorf("value for Scalar OID `%s` not found in `%v`", oid, v.scalarValues)
	}
	return value, nil
}

func (v *resultValueStore) getColumnValues(oid string) (map[string]snmpValueType, error) {
	values, ok := v.columnValues[oid]
	if !ok {
		return nil, fmt.Errorf("value for Column OID `%s` not found in `%v`", oid, v.columnValues)
	}
	retValues := make(map[string]snmpValueType, len(values))
	for index, value := range values {
		retValues[index] = value
	}

	return retValues, nil
}
