// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_makeStringBatches(t *testing.T) {
	tests := []struct {
		name            string
		elements        []string
		size            int
		expectedBatches [][]string
		expectedError   error
	}{
		{
			"three batches, last with diff length",
			[]string{"aa", "bb", "cc", "dd", "ee"},
			2,
			[][]string{
				{"aa", "bb"},
				{"cc", "dd"},
				{"ee"},
			},
			nil,
		},
		{
			"two batches same length",
			[]string{"aa", "bb", "cc", "dd", "ee", "ff"},
			3,
			[][]string{
				{"aa", "bb", "cc"},
				{"dd", "ee", "ff"},
			},
			nil,
		},
		{
			"one full batch",
			[]string{"aa", "bb", "cc"},
			3,
			[][]string{
				{"aa", "bb", "cc"},
			},
			nil,
		},
		{
			"one partial batch",
			[]string{"aa"},
			3,
			[][]string{
				{"aa"},
			},
			nil,
		},
		{
			"large batch size",
			[]string{"aa", "bb", "cc", "dd", "ee", "ff"},
			100,
			[][]string{
				{"aa", "bb", "cc", "dd", "ee", "ff"},
			},
			nil,
		},
		{
			"zero element",
			[]string{},
			2,
			[][]string(nil),
			nil,
		},
		{
			"zero batch size",
			[]string{"aa", "bb", "cc", "dd", "ee"},
			0,
			nil,
			fmt.Errorf("batch size must be positive. invalid size: 0"),
		},
		{
			"negative batch size",
			[]string{"aa", "bb", "cc", "dd", "ee"},
			-1,
			nil,
			fmt.Errorf("batch size must be positive. invalid size: -1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches, err := CreateStringBatches(tt.elements, tt.size)
			assert.Equal(t, tt.expectedBatches, batches)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}
