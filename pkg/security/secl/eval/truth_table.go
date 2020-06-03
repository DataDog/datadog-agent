package eval

import (
	"math/rand"
	"reflect"

	"github.com/pkg/errors"
)

type truthEntry struct {
	Values FilterValues
	Result bool
}

type truthTable struct {
	Entries []truthEntry
}

func (tt *truthTable) getApprovers(fields ...string) map[string]FilterValues {
	filterValues := make(map[string]FilterValues)

	for _, entry := range tt.Entries {
		if !entry.Result {
			continue
		}

		// in order to have approvers we need to ensure that for a "true" result
		// we always have all the given field set to true. If we find a "true" result
		// with a field set to false we can exclude the given fields as approvers.
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
				if !value.ignore && field == value.Field {
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

func newTruthTable(rule *Rule, model Model, event Event) (*truthTable, error) {
	model.SetEvent(event)

	var filterValues []*FilterValue
	for field, fValues := range rule.evaluator.FieldValues {
		// case where there is no static value, ex: process.gid == process.uid
		// so generate fake value in order to be able to get the truth table
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
				return nil, &FieldTypeUnknown{Field: field}
			}

			filterValues = append(filterValues, &FilterValue{
				Field:  field,
				Value:  value,
				Type:   ScalarValueType,
				ignore: true,
			})

			continue
		}

		for _, fValue := range fValues {
			filterValues = append(filterValues, &FilterValue{
				Field: field,
				Value: fValue.Value,
				Type:  fValue.Type,
			})
		}
	}

	if len(filterValues) == 0 {
		return nil, nil
	}

	if len(filterValues) >= 64 {
		return nil, errors.New("limit of field values reached")
	}

	var truthTable truthTable
	for i := 0; i < (1 << len(filterValues)); i++ {
		var entry truthEntry

		for j, value := range filterValues {
			value.Not = (i & (1 << j)) > 0
			if value.Not {
				notValue, err := notOfValue(value.Value)
				if err != nil {
					return nil, &ValueTypeUnknown{Field: value.Field}
				}
				event.SetFieldValue(value.Field, notValue)
			} else {
				event.SetFieldValue(value.Field, value.Value)
			}

			entry.Values = append(entry.Values, FilterValue{
				Field: value.Field,
				Value: value.Value,
				Type:  value.Type,
				Not:   value.Not,
			})
		}
		entry.Result = rule.evaluator.Eval(&Context{})

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
