// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBool(t *testing.T) {
	cases := []struct {
		value         interface{}
		expected      bool
		expectedError bool
	}{
		{true, true, false},
		{false, false, false},
		{"true", true, false},
		{"false", false, false},
		{"invalid", false, true},
		{1, false, true},
		{nil, false, true},
	}

	for _, c := range cases {
		v, err := GetBool(c.value)
		if c.expectedError {
			assert.NotNil(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, c.expected, v)
		}
	}
}

func TestGetInt(t *testing.T) {
	cases := []struct {
		value         interface{}
		expected      int
		expectedError bool
	}{
		{0, 0, false},
		{1, 1, false},
		{-1, -1, false},
		{0x7fff_ffff, 0x7fff_ffff, false},
		{-0x7fff_ffff, -0x7fff_ffff, false},
		{"0", 0, false},
		{"1", 1, false},
		{"-1", -1, false},
		{"2147483647", 2147483647, false},
		{"-2147483648", -2147483648, false},
		{"0x1", 0, true},
		{"aaa", 0, true},
	}

	for _, c := range cases {
		v, err := GetInt(c.value)
		if c.expectedError {
			assert.NotNil(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, v, c.expected)
		}
	}
}
