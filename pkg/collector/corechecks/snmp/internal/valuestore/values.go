// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OIDNotFoundError is returned when an OID cannot be found in the value store
type OIDNotFoundError struct {
	OID   string
	Index string // optional, empty if not applicable
}

func (e *OIDNotFoundError) Error() string {
	if e.Index == "" {
		return fmt.Sprintf("OID %s not found", e.OID)
	}
	return fmt.Sprintf("OID %s index %s not found", e.OID, e.Index)
}

// ListOIDs formats a list of *OIDNotFoundErrors concisely for debug logging.
// The output will simply be a space-separated list of OIDs, with '#<index>'
// appended to any that were missing an index, e.g. [1.2.3 1.2.4#22 1.4.5]
func ListOIDs(errs []*OIDNotFoundError) string {
	var b strings.Builder
	b.WriteString("[")
	for i, err := range errs {
		b.WriteString(err.OID)
		if err.Index != "" {
			b.WriteString("#" + err.Index)
		}
		if i < len(errs)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString("]")
	return b.String()
}

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

// ContainsScalarValue returns whether a scalar oid is present in ResultValueStore
func (v *ResultValueStore) ContainsScalarValue(oid string) bool {
	_, ok := v.ScalarValues[oid]
	return ok
}

// GetScalarValue look for oid in ResultValueStore and returns the value and boolean
// weather valid value has been found
func (v *ResultValueStore) GetScalarValue(oid string) (ResultValue, error) {
	value, ok := v.ScalarValues[oid]
	if !ok {
		return ResultValue{}, &OIDNotFoundError{OID: oid}
	}
	return value, nil
}

// ContainsColumnValues returns whether a column oid is present in ResultValueStore
func (v *ResultValueStore) ContainsColumnValues(oid string) bool {
	_, ok := v.ColumnValues[oid]
	return ok
}

// GetColumnValues look for oid in ResultValueStore and returns a map[<fullIndex>]ResultValue
// where `fullIndex` refer to the entire index part of the instance OID.
// For example if the row oid (instance oid) is `1.3.6.1.4.1.1.2.3.10.11.12`,
// the column oid is `1.3.6.1.4.1.1.2.3`, the fullIndex is `10.11.12`.
func (v *ResultValueStore) GetColumnValues(oid string) (map[string]ResultValue, error) {
	values, ok := v.ColumnValues[oid]
	if !ok {
		return nil, &OIDNotFoundError{OID: oid}
	}
	retValues := make(map[string]ResultValue, len(values))
	maps.Copy(retValues, values)

	return retValues, nil
}

// getColumnValue look for oid in ResultValueStore and returns a ResultValue
func (v *ResultValueStore) getColumnValue(oid string, index string) (ResultValue, error) {
	values, ok := v.ColumnValues[oid]
	if !ok {
		return ResultValue{}, &OIDNotFoundError{OID: oid}
	}
	value, ok := values[index]
	if !ok {
		return ResultValue{}, &OIDNotFoundError{OID: oid, Index: index}
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
		if errors.Is(err, &OIDNotFoundError{}) {
			return nil, err
		}
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
