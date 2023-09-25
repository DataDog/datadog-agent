package profiledefinition

import (
	"encoding/json"
)

//
//type Tag struct {
//	Name string `json:"name"`
//}
//
//type Foo struct {
//	Tags JSONListMap[Tag] `json:"tags"`
//}

type JSONListMap[T any] map[string]T

type MapItem[T any] struct {
	Key   string `json:"key"`
	Value T      `json:"value"`
}

// MarshalJSON TODO
func (jlm JSONListMap[T]) MarshalJSON() ([]byte, error) {
	var items []MapItem[T]
	for key, value := range jlm {
		items = append(items, MapItem[T]{Key: key, Value: value})
	}
	return json.Marshal(items)
}

// UnmarshalJSON TODO
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
