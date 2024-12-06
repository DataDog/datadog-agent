// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateCmdline(t *testing.T) {
	type testData struct {
		original []string
		result   []string
	}

	tests := []testData{
		{
			original: []string{},
			result:   nil,
		},
		{
			original: []string{"a", "b", "", "c", "d"},
			result:   []string{"a", "b", "c", "d"},
		},
		{
			original: []string{"x", strings.Repeat("A", maxCommandLine-1)},
			result:   []string{"x", strings.Repeat("A", maxCommandLine-1)},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine), "B"},
			result:   []string{strings.Repeat("A", maxCommandLine)},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine+1)},
			result:   []string{strings.Repeat("A", maxCommandLine)},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine-1), "", "B"},
			result:   []string{strings.Repeat("A", maxCommandLine-1), "B"},
		},
		{
			original: []string{strings.Repeat("A", maxCommandLine-1), "BCD"},
			result:   []string{strings.Repeat("A", maxCommandLine-1), "B"},
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.result, truncateCmdline(test.original))
	}
}
