package testinit

import (
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

func TestInit(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	if err := runInit(); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}

	// Check for expected allocations
	helpers.AssertMemoryExpectation(t, helpers.Allocations, initAllocations)
}
