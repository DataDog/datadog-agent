package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

type FieldCapabilities []FieldCapability

type FieldCapability struct {
	Field eval.Field
	Types eval.FieldValueType
}

func (fcs FieldCapabilities) GetFields() []eval.Field {
	var fields []eval.Field
	for _, fc := range fcs {
		fields = append(fields, fc.Field)
	}
	return fields
}

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
