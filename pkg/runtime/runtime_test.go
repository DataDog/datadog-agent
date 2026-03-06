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
			maxProcsValue: "1000m",
			expected:      1,
		},
		{
			maxProcsValue: "1500m",
			expected:      1,
		},
		{
			maxProcsValue: "2000m",
			expected:      2,
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

func TestNumVCPU(t *testing.T) {
	// NumVCPU should return the current GOMAXPROCS value
	originalProcs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(originalProcs)

	runtime.GOMAXPROCS(4)
	assert.Equal(t, 4, NumVCPU())

	runtime.GOMAXPROCS(1)
	assert.Equal(t, 1, NumVCPU())
}

func TestSetMaxProcs_EmptyString(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	t.Setenv("GOMAXPROCS", "")
	result := SetMaxProcs()
	// Should return after logging error about empty string
	assert.True(t, result)
}

func TestSetMaxProcs_ValidInteger(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	t.Setenv("GOMAXPROCS", "4")
	result := SetMaxProcs()
	// Should return true - automaxprocs runs first, then we check env var
	// The valid integer path returns early without error
	assert.True(t, result)
}

func TestSetMaxProcs_InvalidMilliCPUs(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	runtime.GOMAXPROCS(2)
	t.Setenv("GOMAXPROCS", "invalidm")
	result := SetMaxProcs()
	// Should log error about invalid milliCPUs parsing
	assert.True(t, result)
}

func TestSetMaxProcs_LowMilliCPUs(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	t.Setenv("GOMAXPROCS", "500m")
	SetMaxProcs()
	// milliCPUs < 1000 should set GOMAXPROCS to 1
	assert.Equal(t, 1, runtime.GOMAXPROCS(0))
}

func TestSetMaxProcs_UnhandledFormat(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	runtime.GOMAXPROCS(2)
	t.Setenv("GOMAXPROCS", "invalid-format")
	result := SetMaxProcs()
	// Should log error about unhandled value
	assert.True(t, result)
}
