// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// FilterMode defines a filter mode
type FilterMode int

const (
	// NormalMode enabled approver and discarder
	NormalMode FilterMode = iota
	// ApproverOnlyMode not used to generate a discarder
	ApproverOnlyMode
)

// FieldCapabilities holds a list of field capabilities
type FieldCapabilities []FieldCapability

// FieldCapability represents a field and the type of its value (scalar, pattern, bitmask, ...)
type FieldCapability struct {
	Field                  eval.Field
	TypeBitmask            eval.FieldValueType
	ValidateFnc            func(FilterValue) bool
	FilterWeight           int
	FilterMode             FilterMode
	RangeFilterValue       *RangeFilterValue
	HandleNotApproverValue func(value interface{}) (interface{}, bool)
}

// TypeMatches return if a type is supported
func (fc FieldCapability) TypeMatches(kind eval.FieldValueType) bool {
	return kind&fc.TypeBitmask != 0
}

// Validate validate the filter value
func (fc FieldCapability) Validate(filterValue FilterValue) bool {
	if filterValue.Field != fc.Field || !fc.TypeMatches(filterValue.Type) {
		return false
	}

	if fc.ValidateFnc != nil {
		if !fc.ValidateFnc(filterValue) {
			return false
		}
	}

	return true
}

// GetFields returns all the fields of FieldCapabilities
func (fcs FieldCapabilities) GetFields() []eval.Field {
	var fields []eval.Field
	for _, fc := range fcs {
		fields = append(fields, fc.Field)
	}
	return fields
}
