// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"context"
	"runtime/debug"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/stretchr/testify/assert"
)

func TestStaticMemoryLimiter(t *testing.T) {
	currentLimit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetMemoryLimit(currentLimit)
	})

	limiter := staticMemoryLimiter{
		limitPct: 0.42,
	}

	// Test no action if already set
	debug.SetMemoryLimit(42 * 1024 * 1024)
	err := limiter.Run(context.Background())
	assert.NoError(t, err)
	assert.EqualValues(t, 42*1024*1024, debug.SetMemoryLimit(-1))

	// Test memory limit is set to 42% of provided limit
	limiter.setMemoryLimit(33 * 1024 * 1024)
	assert.EqualValues(t, 0.42*33*1024*1024, debug.SetMemoryLimit(-1))
}

func TestDynamicMemoryLimiter(t *testing.T) {
	currentLimit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetMemoryLimit(currentLimit)
	})

	memoryLimit := 42 * 1024 * 1024
	mockCgroup := &cgroups.MockCgroup{
		Memory: &cgroups.MemoryStats{
			Limit: pointer.Ptr(uint64(memoryLimit)),
		},
	}

	externalMemoryValue := uint64(1024 * 1024)

	limiter := dynamicMemoryLimiter{
		selfCgroup:  mockCgroup,
		minLimitPct: 0.20,
		externalMemoryReader: func(cgroups.MemoryStats) uint64 {
			return externalMemoryValue
		},
	}

	// Testing limiter takes externalMemoryReader
	limiter.computeSetLimit()
	assert.EqualValues(t, 41*1024*1024, debug.SetMemoryLimit(-1))

	// Test that previously set limit does not impact calculation
	externalMemoryValue = uint64(2 * 1024 * 1024)
	limiter.computeSetLimit()
	assert.EqualValues(t, 40*1024*1024, debug.SetMemoryLimit(-1))

	// Test that we can't go below minLimitPct
	externalMemoryValue = uint64(42 * 1024 * 1024)
	limiter.computeSetLimit()
	assert.EqualValues(t, limiter.minLimitPct*float64(memoryLimit), debug.SetMemoryLimit(-1))
}
