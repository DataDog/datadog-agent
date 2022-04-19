// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Approvers are just filter values indexed by field
type Approvers map[eval.Field]FilterValues

// isAnApprover returns whether the given value is an approver for the given rule
func isAnApprover(event eval.Event, ctx *eval.Context, rule *Rule, field eval.Field, value interface{}) (bool, error) {
	if err := event.SetFieldValue(field, value); err != nil {
		return false, err
	}
	origResult, err := rule.PartialEval(ctx, field)
	if err != nil {
		return false, err
	}

	notValue, err := eval.NotOfValue(value)
	if err != nil {
		return false, err
	}

	if err := event.SetFieldValue(field, notValue); err != nil {
		return false, err
	}
	notResult, err := rule.PartialEval(ctx, field)
	if err != nil {
		return false, err
	}

	if origResult && !notResult {
		return true, nil
	}

	return false, nil
}

func bitmaskCombinations(bitmasks []int) []int {
	if len(bitmasks) == 0 {
		return nil
	}

	var result []int
	for i := 0; i < (1 << len(bitmasks)); i++ {
		var mask int
		for j, value := range bitmasks {
			if (i & (1 << j)) > 0 {
				mask |= value
			}
		}
		result = append(result, mask)
	}

	return result
}

// GetApprovers returns approvers for the given rules
func GetApprovers(rules []*Rule, event eval.Event, fieldCaps FieldCapabilities) (Approvers, error) {
	approvers := make(Approvers)

	ctx := eval.NewContext(event.GetPointer())

	// for each rule we should at least find one approver otherwise we will return no approver for the field
	for _, rule := range rules {
		var bestFilterField eval.Field
		var bestFilterValues FilterValues
		var bestFilterWeight int

	LOOP:
		for _, fieldCap := range fieldCaps {
			field := fieldCap.Field

			var filterValues FilterValues
			var bitmasks []int

			for _, value := range rule.GetFieldValues(field) {
				switch value.Type {
				case eval.ScalarValueType, eval.PatternValueType, eval.GlobValueType:
					isAnApprover, err := isAnApprover(event, ctx, rule, field, value.Value)
					if err != nil {
						return nil, err
					}

					if isAnApprover {
						filterValues = filterValues.Merge(FilterValue{Field: field, Value: value.Value, Type: value.Type})
					} else if fieldCap.Types&eval.BitmaskValueType == 0 {
						// if not a bitmask we need to have all the value as approvers
						// basically a list of values ex: in ["test123", "test456"]
						continue LOOP
					}
				case eval.BitmaskValueType:
					bitmasks = append(bitmasks, value.Value.(int))
				}
			}

			for _, bitmask := range bitmaskCombinations(bitmasks) {
				isAnApprover, err := isAnApprover(event, ctx, rule, field, bitmask)
				if err != nil {
					return nil, err
				}

				if isAnApprover {
					filterValues = filterValues.Merge(FilterValue{Field: field, Value: bitmask, Type: eval.BitmaskValueType})
				}
			}

			if len(filterValues) == 0 || !fieldCaps.Validate(filterValues) {
				continue
			}

			if bestFilterValues == nil || fieldCap.FilterWeight > bestFilterWeight {
				bestFilterField = field
				bestFilterValues = filterValues
				bestFilterWeight = fieldCap.FilterWeight
			}
		}

		// no filter value for a rule thus no approver for the event type
		if bestFilterValues == nil {
			return nil, nil
		}

		approvers[bestFilterField] = append(approvers[bestFilterField], bestFilterValues...)
	}

	return approvers, nil
}
