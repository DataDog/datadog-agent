// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
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
		privateValue float64 //nolint:structcheck
		JSONLessStr  string
	}

	type Nested struct {
		Foo []MoreNested `json:"moreNested"`
	}

	type Top struct {
		Name      string  `json:"name"`
		Value     int     `json:"value"`
		NestedPtr *Nested `json:"nested"`
		ID        string  `json:"-"`
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
	}

	assert.Equal(t, StructToMap(top), map[string]interface{}{
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
	})
}
