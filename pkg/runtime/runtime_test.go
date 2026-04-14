// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runtime

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutoMaxProcs(t *testing.T) {

	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	// let's change at runtime to 2 threads
	runtime.GOMAXPROCS(2)
	assert.Equal(t, 2, runtime.GOMAXPROCS(0))

	tests := []struct {
		maxProcsValue string
		expected      int
	}{
		{
			maxProcsValue: "500m",
			expected:      5,
		},
		{
			maxProcsValue: "1000m",
			expected:      5,
		},
		{
			maxProcsValue: "1500m",
			expected:      5,
		},
		{
			maxProcsValue: "1999m",
			expected:      5,
		},
		{
			maxProcsValue: "2000m",
			expected:      5,
		},
		{
			maxProcsValue: "3000m",
			expected:      5,
		},
		{
			maxProcsValue: "4999m",
			expected:      5,
		},
		{
			maxProcsValue: "5000m",
			expected:      5,
		},
		{
			maxProcsValue: "6000m",
			expected:      6,
		},
	}
	for _, test := range tests {
		t.Run(test.maxProcsValue, func(t *testing.T) {
			t.Setenv("GOMAXPROCS", test.maxProcsValue)
			// set new limit
			SetMaxProcs()
			assert.Equal(t, test.expected, runtime.GOMAXPROCS(0))
		})
	}
}
