// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var vals = OptPairs[int]{
	{"", 1},
	{"TWO", 2},
	{"three", 3},
	{"fOUr", 4},
}

var testCases = []struct {
	choice      string
	expectedKey string
	expectedVal int
	ok          bool
}{
	{"TWO", "TWO", 2, true},
	{"two", "TWO", 2, true},
	{"three", "three", 3, true},
	{"THREE", "three", 3, true},
	{"FouR", "fOUr", 4, true},
	{"", "", 1, true},
	{"five", "", 0, false},
}

func TestOptions(t *testing.T) {
	m := NewOptions(vals)

	assert.Equal(t, "TWO|three|fOUr", m.OptsStr())
	for _, tc := range testCases {
		gotKey, ok := m.getOpt(tc.choice)
		assert.Equal(t, tc.ok, ok)
		if ok {
			assert.Equal(t, tc.expectedKey, gotKey)
		}
		gotVal, ok := m.getVal(tc.choice)
		assert.Equal(t, tc.ok, ok)
		if ok {
			assert.Equal(t, tc.expectedVal, gotVal)
		}
	}
}

func TestOptsFlag(t *testing.T) {
	m := NewOptions(vals)
	var s string
	flag := m.Flag(&s)

	assert.Equal(t, flag.String(), "")
	assert.Equal(t, flag.Type(), "option")

	for _, tc := range testCases {
		if !tc.ok {
			old := s
			err := flag.Set(tc.choice)
			assert.ErrorContains(t, err, "TWO|three|fOUr")
			assert.Equal(t, s, old)
		} else {
			assert.NoError(t, flag.Set(tc.choice))
			assert.Equal(t, tc.expectedKey, s)
			assert.Equal(t, tc.expectedKey, flag.String())
		}
	}
}
