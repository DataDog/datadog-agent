// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

func TestInit(t *testing.T) {
	// Test failed initialization first
	helpers.ResetMemoryStats()
	rtloader, err := getRtLoader()
	require.NoError(t, err, "Failed to get rtloader")

	err = runInit(rtloader, "../invalid/path")
	assert.Error(t, err, "Expected error for invalid path")
	t.Logf("Error: %v", err)

	// Check for leaks
	helpers.AssertMemoryUsage(t)

	// Test successful initialization with a new rtloader instance
	helpers.ResetMemoryStats()
	//rtloader, err = getRtLoader()
	//require.NoError(t, err, "Failed to get rtloader")

	err = runInit(rtloader, "../python")
	assert.NoError(t, err, "Expected successful initialization")

	// Check for expected allocations
	helpers.AssertMemoryExpectation(t, helpers.Allocations, initAllocations)
}
