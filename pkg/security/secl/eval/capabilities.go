package eval

type FieldCapabilities []FieldCapability

type FieldCapability struct {
	Field string
	Types FieldValueType
}

func (fcs FieldCapabilities) GetFields() []string {
	var fields []string
	for _, fc := range fcs {
		fields = append(fields, fc.Field)
	}
	return fields
}

func (fcs FieldCapabilities) Validate(approvers map[string]FilterValues) bool {
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
