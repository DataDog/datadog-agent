// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type myStruct struct {
	Name string
}

type Identifier string

func TestValidateBasic(t *testing.T) {
	type testCase struct {
		name   string
		val    interface{}
		expect bool
	}

	cases := []testCase{
		{
			name:   "basic string",
			val:    "cat",
			expect: true,
		},
		{
			name:   "basic int",
			val:    123,
			expect: true,
		},
		{
			name:   "basic float",
			val:    4.21,
			expect: true,
		},
		{
			name:   "basic bool",
			val:    false,
			expect: true,
		},
		{
			name:   "slice of string",
			val:    []string{"dog"},
			expect: true,
		},
		{
			name:   "map of string",
			val:    map[string]string{"eel": "zap"},
			expect: true,
		},
		{
			name:   "map of string typed as interface",
			val:    map[Identifier]interface{}{"frog": "ribbit"},
			expect: true,
		},
		{
			name:   "map of type alias",
			val:    map[Identifier]string{Identifier("bad"): "kind"},
			expect: true,
		},
		{
			name:   "error struct",
			val:    myStruct{Name: "bob"},
			expect: false,
		},
		{
			name:   "error map of slice",
			val:    []myStruct{{Name: "bob"}},
			expect: false,
		},
		{
			name:   "error map of struct",
			val:    map[string]myStruct{"first": {Name: "bob"}},
			expect: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := ValidateBasicTypes(c.val)
			assert.Equal(t, c.expect, res)
		})
	}
}
