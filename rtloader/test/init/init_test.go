package testinit

import (
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

func TestInit(t *testing.T) {
	if err := runInit(); err != nil {
		t.Errorf("Expected nil, got: %v", err)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
