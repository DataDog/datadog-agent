// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Store MetadataStore stores metadata scalarValues
type Store struct {
	// map[<FIELD>]valuestore.ResultValue
	scalarValues map[string]valuestore.ResultValue

	// map[<FIELD>][<index>]valuestore.ResultValue
	columnValues map[string]map[string]valuestore.ResultValue

	// map[<RESOURCE>][<index>][]<TAG>
	resourceIDTags map[string]map[string][]string
}

// NewMetadataStore returns a new metadata Store
func NewMetadataStore() *Store {
	return &Store{
		scalarValues:   make(map[string]valuestore.ResultValue),
		columnValues:   make(map[string]map[string]valuestore.ResultValue),
		resourceIDTags: make(map[string]map[string][]string),
	}
}

// AddScalarValue add scalar value to metadata store
func (s Store) AddScalarValue(field string, value valuestore.ResultValue) {
	s.scalarValues[field] = value
}

// AddColumnValue add column value to metadata store
func (s Store) AddColumnValue(field string, index string, value valuestore.ResultValue) {
	_, ok := s.columnValues[field]
	if !ok {
		s.columnValues[field] = make(map[string]valuestore.ResultValue)
	}
	s.columnValues[field][index] = value
}

// GetColumnAsString get column value as string
func (s Store) GetColumnAsString(field string, index string) string {
	column, ok := s.columnValues[field]
	if !ok {
		return ""
	}
	value, ok := column[index]
	if !ok {
		return ""
	}
	strVal, err := value.ToString()
	if err != nil {
		log.Debugf("error converting value string `%v`: %s", value, err)
		return ""
	}
	return strVal
}

// GetColumnAsByteArray get column value as []byte
func (s Store) GetColumnAsByteArray(field string, index string) []byte {
	column, ok := s.columnValues[field]
	if !ok {
		return nil
	}
	value, ok := column[index]
	if !ok {
		return nil
	}
	switch value.Value.(type) {
	case []byte:
		return value.Value.([]byte)
	}
	return nil
}

// GetColumnAsFloat get column value as float
func (s Store) GetColumnAsFloat(field string, index string) float64 {
	column, ok := s.columnValues[field]
	if !ok {
		return 0
	}
	value, ok := column[index]
	if !ok {
		return 0
	}
	strVal, err := value.ToFloat64()
	if err != nil {
		log.Debugf("error converting value to float `%v`: %s", value, err)
		return 0
	}
	return strVal
}

// GetScalarAsString get scalar value as string
func (s Store) GetScalarAsString(field string) string {
	value, ok := s.scalarValues[field]
	if !ok {
		return ""
	}
	strVal, err := value.ToString()
	if err != nil {
		log.Debugf("error parsing value `%v`: %s", value, err)
		return ""
	}
	return strVal
}

// ScalarFieldHasValue test if scalar field has value
func (s Store) ScalarFieldHasValue(field string) bool {
	_, ok := s.scalarValues[field]
	return ok
}

// GetColumnIndexes get column indexes for a field
func (s Store) GetColumnIndexes(field string) []string {
	column, ok := s.columnValues[field]
	if !ok {
		return nil
	}
	var indexes []string
	for key := range column {
		indexes = append(indexes, key)
	}
	return indexes
}

// GetIDTags get idTags for a specific resource and index
func (s Store) GetIDTags(resource string, index string) []string {
	resTags, ok := s.resourceIDTags[resource]
	if !ok {
		return nil
	}
	tags, ok := resTags[index]
	if !ok {
		return nil
	}
	return tags
}

// AddIDTags add idTags for a specific resource and index
func (s Store) AddIDTags(resource string, index string, tags []string) {
	_, ok := s.resourceIDTags[resource]
	if !ok {
		s.resourceIDTags[resource] = make(map[string][]string)
	}
	s.resourceIDTags[resource][index] = append(s.resourceIDTags[resource][index], tags...)
}
