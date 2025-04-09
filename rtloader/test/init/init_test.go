// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinit

import (
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

func TestInitFailure(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	if err := runInit("../invalid/path"); err == nil {
		t.Errorf("Expected error, got nil")
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestInit(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	if err := runInit("../python"); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}

	// Check for expected allocations
	helpers.AssertMemoryExpectation(t, helpers.Allocations, initAllocations)
}
