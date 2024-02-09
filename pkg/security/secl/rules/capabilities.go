// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// FieldCapabilities holds a list of field capabilities
type FieldCapabilities []FieldCapability

// FieldCapability represents a field and the type of its value (scalar, pattern, bitmask, ...)
type FieldCapability struct {
	Field        eval.Field
	Types        eval.FieldValueType
	ValidateFnc  func(FilterValue) bool
	FilterWeight int
}

// GetFields returns all the fields of FieldCapabilities
func (fcs FieldCapabilities) GetFields() []eval.Field {
	var fields []eval.Field
	for _, fc := range fcs {
		fields = append(fields, fc.Field)
	}
	return fields
}

// Validate ensures all the filter values match field capabilities
func (fcs FieldCapabilities) Validate(filterValues FilterValues) bool {
	for _, filterValue := range filterValues {
		var found bool
		for _, fc := range fcs {
			if filterValue.Field != fc.Field || filterValue.Type&fc.Types == 0 {
				continue
			}

			if fc.ValidateFnc != nil {
				if !fc.ValidateFnc(filterValue) {
					continue
				}
			}

			found = true
			break
		}

		if !found {
			return false
		}
	}

	return true
}
