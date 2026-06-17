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

	// Simulate automaxprocs resolving to 1 (e.g. <=1 vCPU cgroup quota on ECS Fargate).
	// SetMaxProcs should raise it to the minimum of 2.
	runtime.GOMAXPROCS(1)
	SetMaxProcs()
	assert.GreaterOrEqual(t, runtime.GOMAXPROCS(0), 2)

	// let's change at runtime to 2 threads
	runtime.GOMAXPROCS(2)
	assert.Equal(t, 2, runtime.GOMAXPROCS(0))

	tests := []struct {
		maxProcsValue string
		expected      int
	}{
		{
			maxProcsValue: "500m",
			expected:      2,
		},
		{
			maxProcsValue: "1000m",
			expected:      2,
		},
		{
			maxProcsValue: "1500m",
			expected:      2,
		},
		{
			maxProcsValue: "1999m",
			expected:      2,
		},
		{
			maxProcsValue: "2000m",
			expected:      2,
		},
		{
			maxProcsValue: "3000m",
			expected:      3,
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
