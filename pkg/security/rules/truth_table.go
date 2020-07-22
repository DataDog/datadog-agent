package rules

import (
	"math/rand"
	"reflect"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type truthEntry struct {
	Values FilterValues
	Result bool
}

type truthTable struct {
	Entries []truthEntry
}

func (tt *truthTable) getApprovers(fields ...string) map[eval.Field]FilterValues {
	filterValues := make(map[eval.Field]FilterValues)

	for _, entry := range tt.Entries {
		if !entry.Result {
			continue
		}

		// a field value can't be an approver if we can find a entry that is true
		// when all the fields are set to false.
		allFalse := true
		for _, field := range fields {
			for _, value := range entry.Values {
				if value.Field == field && !value.Not {
					allFalse = false
					break
				}
			}
		}

		if allFalse {
			return nil
		}

		for _, field := range fields {
		LOOP:
			for _, value := range entry.Values {
				if !value.ignore && !value.Not && field == value.Field {
					fvs := filterValues[value.Field]
					for _, fv := range fvs {
						// do not append twice the same value
						if fv.Value == value.Value {
							continue LOOP
						}
					}
					fvs = append(fvs, value)
					filterValues[value.Field] = fvs
				}
			}
		}
	}

	return filterValues
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
	for field, fValues := range rule.GetEvaluator().FieldValues {
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
					Field:  field,
					Value:  value,
					Type:   eval.ScalarValueType,
					ignore: true,
				},
			})

			continue
		}

		var bitmasks []int

		var values FilterValues
		for _, fValue := range fValues {
			switch fValue.Type {
			case eval.ScalarValueType, eval.PatternValueType:
				values = append(values, FilterValue{
					Field: field,
					Value: fValue.Value,
					Type:  fValue.Type,
				})

				notValue, err := notOfValue(fValue.Value)
				if err != nil {
					return nil, &ErrValueTypeUnknown{Field: field}
				}

				values = append(values, FilterValue{
					Field: field,
					Value: notValue,
					Type:  fValue.Type,
					Not:   true,
				})
			case eval.BitmaskValueType:
				bitmasks = append(bitmasks, fValue.Value.(int))
			}
		}

		// add combinations of bitmask if bitmasks are used
		if len(bitmasks) > 0 {
			for _, mask := range combineBitmasks(bitmasks) {
				values = append(values, FilterValue{
					Field: field,
					Value: mask,
					Type:  eval.BitmaskValueType,
					Not:   mask == 0,
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

func newTruthTable(rule *eval.Rule, model eval.Model, event eval.Event) (*truthTable, error) {
	model.SetEvent(event)

	if len(rule.GetEvaluator().FieldValues) == 0 {
		return nil, nil
	}

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
				Field:  filterValue.Field,
				Value:  filterValue.Value,
				Type:   filterValue.Type,
				Not:    filterValue.Not,
				ignore: filterValue.ignore,
			})
		}

		entry.Result = rule.GetEvaluator().Eval(&eval.Context{})

		truthTable.Entries = append(truthTable.Entries, entry)
	}

	return &truthTable, nil
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func notOfValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return ^v, nil
	case string:
		return randStringRunes(256), nil
	case bool:
		return !v, nil
	}

	return nil, errors.New("value type unknown")
}
