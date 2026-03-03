// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBitmaskCombinations(t *testing.T) {
	testCases := []struct {
		values               []int
		expectedCombinaisons []int
	}{
		{
			values:               []int{},
			expectedCombinaisons: []int{},
		},
		{
			values:               []int{0},
			expectedCombinaisons: []int{0},
		},
		{
			values:               []int{1},
			expectedCombinaisons: []int{1},
		},
		{
			values:               []int{1, 2},
			expectedCombinaisons: []int{1, 2, 1 | 2},
		},
		{
			values:               []int{1, 2, 4},
			expectedCombinaisons: []int{1, 2, 4, 1 | 2, 1 | 4, 2 | 4, 1 | 2 | 4},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			result := bitmaskCombinations(testCase.values)
			assert.ElementsMatch(t, result, testCase.expectedCombinaisons)
		})
	}
}
