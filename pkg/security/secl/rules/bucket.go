// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"sort"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Approvers are just filter values indexed by field
type Approvers map[eval.Field]FilterValues

// RuleBucket groups rules with the same event type
type RuleBucket struct {
	rules  []*Rule
	fields []eval.Field
}

// AddRule adds a rule to the bucket
func (rb *RuleBucket) AddRule(rule *Rule) error {
	for _, r := range rb.rules {
		if r.ID == rule.ID {
			return &ErrRuleLoad{Definition: rule.Definition, Err: errors.New("multiple definition with the same ID")}
		}
	}

	for _, field := range rule.GetEvaluator().GetFields() {
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

// GetRules returns the bucket rules
func (rb *RuleBucket) GetRules() []*Rule {
	return rb.rules
}

// FieldCombinations - array all the combinations of field
type FieldCombinations [][]eval.Field

func (a FieldCombinations) Len() int           { return len(a) }
func (a FieldCombinations) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a FieldCombinations) Less(i, j int) bool { return len(a[i]) < len(a[j]) }

func fieldCombinations(fields []eval.Field) FieldCombinations {
	var result FieldCombinations

	for i := 1; i < (1 << len(fields)); i++ {
		var subResult []eval.Field
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

func getFilterValuesWeight(filterValues FilterValues, fieldCaps FieldCapabilities) int {
	var maxWeight int
	for _, filterValue := range filterValues {
		for _, fc := range fieldCaps {
			if filterValue.Field == fc.Field && fc.FilterWeight > maxWeight {
				maxWeight = fc.FilterWeight
			}
		}
	}
	return maxWeight
}

// GetApprovers returns the approvers for an event
func (rb *RuleBucket) GetApprovers(event eval.Event, fieldCaps FieldCapabilities) (Approvers, error) {
	fcs := fieldCombinations(fieldCaps.GetFields())

	approvers := make(Approvers)
	for _, rule := range rb.rules {
		truthTable, err := newTruthTable(rule.Rule, event)
		if err != nil {
			return nil, err
		}

		var filterValues FilterValues
		var maxWeight int

		for _, fields := range fcs {
			candidates := truthTable.getApprovers(fields...)

			// only one combination of the approvers is required, keep the best one meaning
			// the one with the highest weight
			if len(candidates) > 0 && fieldCaps.Validate(candidates) {
				// as fieldCombinations return all the combinations in the less fields order
				// we keep the highest weight with a minimal set of fields.
				if weight := getFilterValuesWeight(candidates, fieldCaps); filterValues == nil || weight > maxWeight {
					filterValues = candidates
					maxWeight = weight
				}
			}
		}

		if len(filterValues) == 0 {
			return nil, &ErrNoApprover{Fields: fieldCaps.GetFields()}
		}

		// dispatch per field
		for _, filterValue := range filterValues {
			approvers[filterValue.Field] = approvers[filterValue.Field].Merge(filterValue)
		}
	}

	return approvers, nil
}
