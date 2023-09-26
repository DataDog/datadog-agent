package profiledefinition

import (
	"encoding/json"
)

// JSONListMap is used to convert map[string]T into a list of key-value.
// This is a workaround for RC that doesn't allow arbitrary properties in json created by the use of map.
type JSONListMap[T any] map[string]T

// MapItem represent one map entry as list item
type MapItem[T any] struct {
	Key   string `json:"key"`
	Value T      `json:"value"`
}

// MarshalJSON marshals to json
func (jlm *JSONListMap[T]) MarshalJSON() ([]byte, error) {
	var items []MapItem[T]
	for key, value := range *jlm {
		items = append(items, MapItem[T]{Key: key, Value: value})
	}
	return json.Marshal(items)
}

// UnmarshalJSON un-marshals from json
func (jlm *JSONListMap[T]) UnmarshalJSON(data []byte) error {
	var items []MapItem[T]
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	result := make(JSONListMap[T])
	for _, item := range items {
		result[item.Key] = item.Value
	}
	*jlm = result
	return nil
}
