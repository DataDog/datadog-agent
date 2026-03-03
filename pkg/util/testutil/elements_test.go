// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElementsMatchFn(t *testing.T) {
	type matcherFns = []func(require.TestingT, int, int)
	matchers := func(expect ...int) (res matcherFns) {
		for _, exp := range expect {
			res = append(res, func(t require.TestingT, _, v int) {
				require.Equal(t, exp, v)
			})
		}
		return
	}

	pass := assert.Empty
	fail := assert.NotEmpty

	type caseDef struct {
		name   string
		assert assert.ValueAssertionFunc
		seq    []int
		match  matcherFns
	}
	cases := []caseDef{
		{"empty", pass, nil, nil},
		{"not empty", fail, []int{1}, nil},
		{"empty not", fail, nil, matchers(1)},
		{"match 1", pass, []int{1}, matchers(1)},
		{"match 2", pass, []int{1, 2}, matchers(1, 2)},
		{"match ↊", pass, []int{2, 1}, matchers(1, 2)},
		{"miss 1", fail, []int{1, 2}, matchers(2)},
		{"miss 2", fail, []int{1, 2}, matchers(1)},
		{"miss 3", fail, []int{1, 2}, matchers(1, 3)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := &collectT{}
			ElementsMatchFn(tc, slices.All(c.seq), c.match...)
			c.assert(t, tc)
		})
	}
}
