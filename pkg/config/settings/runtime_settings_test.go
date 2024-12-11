// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetInt(t *testing.T) {
	cases := []struct {
		v   interface{}
		exp int
		err bool
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
		v, err := GetInt(c.v)
		if c.err {
			assert.NotNil(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, v, c.exp)
		}
	}
}
