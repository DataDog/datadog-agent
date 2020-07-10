package rules

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// Approvers associates field names with their Filter values
type Approvers map[eval.Field]FilterValues

// FilterValues - list of FilterValue
type FilterValues []FilterValue

type FilterValue struct {
	Field eval.Field
	Value interface{}
	Type  eval.FieldValueType
	Not   bool

	ignore bool
}

// Merge merges to FilterValues ensuring there is no duplicate value
func (fv FilterValues) Merge(n FilterValues) FilterValues {
LOOP:
	for _, v1 := range n {
		for _, v2 := range fv {
			if v1.Value == v2.Value {
				continue LOOP
			}
		}
		fv = append(fv, v1)
	}

	return fv
}
