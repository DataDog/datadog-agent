// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinit

import (
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

func TestInit(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	rtloader, err := getRtLoader()

	// Test failed initialization first
	err = runInit(rtloader, "../invalid/path")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	t.Logf("Error: %v", err)

	// Check for leaks
	helpers.AssertMemoryUsage(t)

	// Test successful initialization
	if err := runInit(rtloader, "../python"); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}

	// Check for expected allocations
	helpers.AssertMemoryExpectation(t, helpers.Allocations, initAllocations)
}
