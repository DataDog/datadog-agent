package snmp

import (
	"fmt"
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
