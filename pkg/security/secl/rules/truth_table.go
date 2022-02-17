// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

type truthEntry struct {
	Values FilterValues
	Result bool
}

type truthTable struct {
	Entries []truthEntry
}

func (tt *truthTable) isAlwaysFalseWith(filterValues ...FilterValue) bool {
LOOP:
	for _, entry := range tt.Entries {
		// keep entries for which all the filterValues are "not". We want to check whether
		// with all the opposive values we can find an entry with a "true" result. In that
		// case it means that the final result doesn't depend on the given field values.
		for _, eValue := range entry.Values {
			for _, fValue := range filterValues {
				if eValue.Field == fValue.Field {
					// since not scalar value we can't determine the result
					if !eValue.isScalar {
						return false
					}

					if eValue.Value != fValue.notValue {
						continue LOOP
					}
				}
			}
		}

		if entry.Result {
			return false
		}
	}
	return true
}

// retrieve positive field value from the truth table for the given fields
func (tt *truthTable) getFieldValues(entry *truthEntry, fields ...string) FilterValues {
	var filterValues FilterValues
	for _, eValue := range entry.Values {
		for _, field := range fields {
			if eValue.Field == field {
				if eValue.not || !eValue.isScalar {
					return nil
				}
				filterValues = filterValues.Merge(eValue)
			}
		}
	}
	return filterValues
}

func (tt *truthTable) getApprovers(fields ...string) FilterValues {
	var allFilterValues FilterValues

	for _, entry := range tt.Entries {
		// consider only True result for later check if an opposite result with the same
		// field values.
		if !entry.Result {
			continue
		}

		filterValues := tt.getFieldValues(&entry, fields...)
		if filterValues == nil {
			continue
		}

		// check whether a result is "true" while having all the fields values set to the
		// "not" value. In that case it means that the field value are not approvers.
		if tt.isAlwaysFalseWith(filterValues...) {
			allFilterValues = allFilterValues.Merge(filterValues...)
		}
	}

	return allFilterValues
}

func combineBitmasks(bitmasks []int) []int {
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

func genFilterValues(rule *eval.Rule, event eval.Event) ([]FilterValues, error) {
	var filterValues []FilterValues
	for _, field := range rule.GetFields() {
		fValues := rule.GetFieldValues(field)

		// case where there is no static value, ex: process.gid == process.uid
		// so generate fake value in order to be able to get the truth table
		// note that we want to have the comparison returning true
		if len(fValues) == 0 {
			var value interface{}

			kind, err := event.GetFieldType(field)
			if err != nil {
				return nil, err
			}
			switch kind {
			case reflect.String:
				value = ""
			case reflect.Int:
				value = 0
			case reflect.Bool:
				value = false
			default:
				return nil, &ErrFieldTypeUnknown{Field: field}
			}

			filterValues = append(filterValues, FilterValues{
				{
					Field:    field,
					Value:    value,
					Type:     eval.ScalarValueType,
					isScalar: false,
				},
			})

			continue
		}

		var bitmasks []int

		var values FilterValues
		for _, fValue := range fValues {
			switch fValue.Type {
			case eval.ScalarValueType, eval.PatternValueType:
				notValue, err := eval.NotOfValue(fValue.Value)
				if err != nil {
					return nil, &ErrValueTypeUnknown{Field: field}
				}

				values = values.Merge(FilterValue{
					Field:    field,
					Value:    fValue.Value,
					Type:     fValue.Type,
					notValue: notValue,
					isScalar: true,
				})

				values = values.Merge(FilterValue{
					Field:    field,
					Value:    notValue,
					Type:     fValue.Type,
					notValue: fValue.Value,
					not:      true,
					isScalar: true,
				})
			case eval.BitmaskValueType:
				bitmasks = append(bitmasks, fValue.Value.(int))
			}
		}

		// add combinations of bitmask if bitmasks are used
		if len(bitmasks) > 0 {
			for _, mask := range combineBitmasks(bitmasks) {
				notValue, err := eval.NotOfValue(mask)
				if err != nil {
					return nil, &ErrValueTypeUnknown{Field: field}
				}

				values = values.Merge(FilterValue{
					Field:    field,
					Value:    mask,
					Type:     eval.BitmaskValueType,
					notValue: notValue,
					isScalar: true,
				})

				values = values.Merge(FilterValue{
					Field:    field,
					Value:    mask,
					Type:     eval.BitmaskValueType,
					notValue: mask,
					not:      true,
					isScalar: true,
				})
			}
		}

		filterValues = append(filterValues, values)
	}

	return filterValues, nil
}

func combineFilterValues(filterValues []FilterValues) []FilterValues {
	combine := func(a []FilterValues, b FilterValues) []FilterValues {
		var result []FilterValues

		for _, va := range a {
			for _, vb := range b {
				var s = make(FilterValues, len(va))
				copy(s, va)
				result = append(result, append(s, vb))
			}
		}

		return result
	}

	var combined []FilterValues
	for _, value := range filterValues[0] {
		combined = append(combined, FilterValues{value})
	}

	for _, values := range filterValues[1:] {
		combined = combine(combined, values)
	}

	return combined
}

func newTruthTable(rule *eval.Rule, event eval.Event) (*truthTable, error) {
	ctx := eval.NewContext(event.GetPointer())

	filterValues, err := genFilterValues(rule, event)
	if err != nil {
		return nil, err
	}

	var truthTable truthTable
	for _, combination := range combineFilterValues(filterValues) {
		var entry truthEntry

		for _, filterValue := range combination {
			if err = event.SetFieldValue(filterValue.Field, filterValue.Value); err != nil {
				return nil, err
			}

			entry.Values = append(entry.Values, FilterValue{
				Field:    filterValue.Field,
				Value:    filterValue.Value,
				Type:     filterValue.Type,
				notValue: filterValue.notValue,
				not:      filterValue.not,
				isScalar: filterValue.isScalar,
			})
		}

		entry.Result = rule.GetEvaluator().Eval(ctx)

		truthTable.Entries = append(truthTable.Entries, entry)
	}

	return &truthTable, nil
}
