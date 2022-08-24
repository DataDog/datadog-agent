// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ColumnResultValuesType is used to store results fetched for column oids
// Structure: map[<COLUMN OIDS AS STRING>]map[<ROW INDEX>]ResultValue
// - the first map key is the table column oid
// - the second map key is the index part of oid (not prefixed with column oid)
type ColumnResultValuesType map[string]map[string]ResultValue

// ScalarResultValuesType is used to store results fetched for scalar oids
// Structure: map[<INSTANCE OID VALUE>]ResultValue
// - the instance oid value (suffixed with `.0`)
type ScalarResultValuesType map[string]ResultValue

// ResultValueStore store OID values
type ResultValueStore struct {
	// TODO: make fields private + use a constructor instead
	ScalarValues ScalarResultValuesType `json:"scalar_values"`
	ColumnValues ColumnResultValuesType `json:"column_values"`
}

// GetScalarValue look for oid in ResultValueStore and returns the value and boolean
// weather valid value has been found
func (v *ResultValueStore) GetScalarValue(oid string) (ResultValue, error) {
	value, ok := v.ScalarValues[oid]
	if !ok {
		return ResultValue{}, fmt.Errorf("value for Scalar OID `%s` not found in results", oid)
	}
	return value, nil
}

// GetColumnValues look for oid in ResultValueStore and returns a map[<fullIndex>]ResultValue
// where `fullIndex` refer to the entire index part of the instance OID.
// For example if the row oid (instance oid) is `1.3.6.1.4.1.1.2.3.10.11.12`,
// the column oid is `1.3.6.1.4.1.1.2.3`, the fullIndex is `10.11.12`.
func (v *ResultValueStore) GetColumnValues(oid string) (map[string]ResultValue, error) {
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

// GetColumnValueAsFloat look for oid/index in ResultValueStore and returns a float64
func (v *ResultValueStore) GetColumnValueAsFloat(oid string, index string) float64 {
	value, err := v.getColumnValue(oid, index)
	if err != nil {
		log.Tracef("failed to get value for OID %s with index %s: %s", oid, index, err)
		return 0
	}
	floatValue, err := value.ToFloat64()
	if err != nil {
		log.Tracef("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return 0
	}
	return floatValue
}

// GetColumnIndexes returns column indexes for a specific column oid
func (v *ResultValueStore) GetColumnIndexes(columnOid string) ([]string, error) {
	indexesMap := make(map[string]struct{})
	metricValues, err := v.GetColumnValues(columnOid)
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

// ResultValueStoreAsString used to format ResultValueStore for debug/trace logging
func ResultValueStoreAsString(values *ResultValueStore) string {
	if values == nil {
		return ""
	}
	jsonPayload, err := json.Marshal(values)
	if err != nil {
		log.Debugf("error marshaling debugVar: %s", err)
		return ""
	}
	return string(jsonPayload)
}
