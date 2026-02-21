// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStringSet(t *testing.T) {
	s := NewStringSet()
	assert.NotNil(t, s)
	assert.Len(t, s, 0)

	s = NewStringSet("a", "b", "b", "c")
	assert.NotNil(t, s)
	assert.Len(t, s, 3)
}

func TestStringSetAdd(t *testing.T) {
	s := NewStringSet()
	s.Add("a")
	assert.Equal(t, []string{"a"}, s.GetAll())
	s.Add("b")
	res := sort.StringSlice(s.GetAll())
	res.Sort()
	assert.Equal(t, []string{"a", "b"}, []string(res))

	s.Add("b")
	res = sort.StringSlice(s.GetAll())
	res.Sort()
	assert.Equal(t, []string{"a", "b"}, []string(res))
}

func TestStringSetGetAll(t *testing.T) {
	s := NewStringSet("a", "b", "b", "c", "c")
	res := sort.StringSlice(s.GetAll())
	res.Sort()
	assert.Equal(t, []string{"a", "b", "c"}, []string(res))
}

func TestStructToMap(t *testing.T) {
	type MoreNested struct {
		Name         string  `json:"name"`
		Value        float64 `json:"value"`
		ID           *string `json:"id"`
		privateValue float64
		JSONLessStr  string
	}

	type Nested struct {
		Foo []MoreNested `json:"moreNested"`
	}

	type Top struct {
		Name      string            `json:"name"`
		Value     int               `json:"value"`
		NestedPtr *Nested           `json:"nested"`
		ID        string            `json:"-"`
		MyMap     map[string]Nested `json:"mymap"`
	}

	str := "toto"
	nested := Nested{
		Foo: []MoreNested{
			{
				Name:         "ms1",
				Value:        1,
				ID:           &str,
				JSONLessStr:  "42",
				privateValue: 1000,
			},
			{
				Name:  "ms2",
				Value: 2,
				ID:    nil,
			},
		},
	}

	top := Top{
		Name:      "top",
		Value:     0,
		NestedPtr: &nested,
		ID:        "top1",
		MyMap: map[string]Nested{
			"n1": nested,
		},
	}

	assert.Equal(t, map[string]interface{}{
		"name":  "top",
		"value": 0,
		"nested": map[string]interface{}{
			"moreNested": []interface{}{
				map[string]interface{}{
					"id":          "toto",
					"name":        "ms1",
					"value":       float64(1),
					"JSONLessStr": "42",
				},
				map[string]interface{}{
					"name":        "ms2",
					"value":       float64(2),
					"JSONLessStr": "",
				},
			},
		},
		"mymap": map[string]interface{}{
			"n1": map[string]interface{}{
				"moreNested": []interface{}{
					map[string]interface{}{
						"id":          "toto",
						"name":        "ms1",
						"value":       float64(1),
						"JSONLessStr": "42",
					},
					map[string]interface{}{
						"name":        "ms2",
						"value":       float64(2),
						"JSONLessStr": "",
					},
				},
			},
		},
	}, StructToMap(top))
}

func TestToPowerOf2(t *testing.T) {
	assert.Equal(t, 1, ToPowerOf2(1))
	assert.Equal(t, 2, ToPowerOf2(2))
	assert.Equal(t, 4, ToPowerOf2(3))
	assert.Equal(t, 4, ToPowerOf2(4))
	assert.Equal(t, 8, ToPowerOf2(7))
	assert.Equal(t, 1024, ToPowerOf2(1024))
	assert.Equal(t, 1024, ToPowerOf2(900))
}

func TestStringSliceTransform(t *testing.T) {
	values := []string{"hello", "world"}
	result := StringSliceTransform(values, func(s string) []byte { return []byte(s) })
	assert.Equal(t, [][]byte{[]byte("hello"), []byte("world")}, result)

	// empty slice
	result = StringSliceTransform([]string{}, func(s string) []byte { return []byte(s) })
	assert.Empty(t, result)
}

func TestGetSliceOfStringMap(t *testing.T) {
	input := []interface{}{
		map[interface{}]interface{}{"key1": "val1", "key2": "val2"},
		map[interface{}]interface{}{"key3": "val3"},
	}
	result, err := GetSliceOfStringMap(input)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "val1", result[0]["key1"])
	assert.Equal(t, "val3", result[1]["key3"])

	// invalid type returns error
	_, err = GetSliceOfStringMap([]interface{}{"not a map"})
	assert.Error(t, err)
}
