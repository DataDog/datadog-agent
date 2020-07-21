package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// FieldCapabilities holds a list of field capabilities
type FieldCapabilities []FieldCapability

// FieldCapability represents a field and the type of its value (scalar, pattern, bitmask, ...)
type FieldCapability struct {
	Field eval.Field
	Types eval.FieldValueType
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
		}
	}

	return true
}
