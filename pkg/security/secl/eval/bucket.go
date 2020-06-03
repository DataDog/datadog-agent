package eval

import "sort"

type RuleBucket struct {
	rules  []*Rule
	fields []string
}

func (rb *RuleBucket) AddRule(rule *Rule) error {
	for _, r := range rb.rules {
		if r.ID == rule.ID {
			return DuplicateRuleID{ID: r.ID}
		}
	}

	for _, field := range rule.evaluator.GetFields() {
		index := sort.SearchStrings(rb.fields, field)
		if index < len(rb.fields) && rb.fields[index] == field {
			continue
		}
		rb.fields = append(rb.fields, "")
		copy(rb.fields[index+1:], rb.fields[index:])
		rb.fields[index] = field
	}

	rb.rules = append(rb.rules, rule)
	return nil
}

func (rb *RuleBucket) GetRules() []*Rule {
	return rb.rules
}

// FieldCombinations - array all the combinations of field
type FieldCombinations [][]string

func (a FieldCombinations) Len() int           { return len(a) }
func (a FieldCombinations) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a FieldCombinations) Less(i, j int) bool { return len(a[i]) < len(a[j]) }

func fieldCombinations(fields []string) FieldCombinations {
	var result FieldCombinations

	for i := 1; i < (1 << len(fields)); i++ {
		var subResult []string
		for j, field := range fields {
			if (i & (1 << j)) > 0 {
				subResult = append(subResult, field)
			}
		}
		result = append(result, subResult)
	}

	// order the list with the single field first
	sort.Sort(result)

	return result
}

func (rb *RuleBucket) GetApprovers(model Model, event Event, fieldCaps FieldCapabilities) (Approvers, error) {
	fcs := fieldCombinations(fieldCaps.GetFields())

	approvers := make(Approvers)
	for _, rule := range rb.rules {
		truthTable, err := newTruthTable(rule, model, event)
		if err != nil {
			return nil, err
		}

		var ruleApprovers map[string]FilterValues
		for _, fields := range fcs {
			ruleApprovers = truthTable.getApprovers(fields...)
			if ruleApprovers != nil && len(ruleApprovers) > 0 && fieldCaps.Validate(ruleApprovers) {
				break
			}
		}

		if ruleApprovers == nil || len(ruleApprovers) == 0 || !fieldCaps.Validate(ruleApprovers) {
			return nil, &NoApprover{Fields: fieldCaps.GetFields()}
		}
		for field, values := range ruleApprovers {
			approvers[field] = approvers[field].Merge(values)
		}
	}

	return approvers, nil
}
