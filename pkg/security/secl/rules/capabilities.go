// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

// Validate ensures that every field has an approver that accepts its type
func (fcs FieldCapabilities) Validate(approvers map[eval.Field]FilterValues) bool {
	for _, fc := range fcs {
		values, exists := approvers[fc.Field]
		if !exists {
			continue
		}

		for _, value := range values {
			if value.Type&fc.Types == 0 {
				return false
			}

			if fc.ValidateFnc != nil {
				if !fc.ValidateFnc(value) {
					return false
				}
			}
		}
	}

	return true
}
