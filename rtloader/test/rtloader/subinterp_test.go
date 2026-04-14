// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package testrtloader

import (
	"testing"

	"github.com/DataDog/datadog-agent/rtloader/test/helpers"
)

// TestHasSubinterpreterSupport verifies that the has_subinterpreter_support
// C API function is callable. The actual return value depends on whether
// rtloader was compiled with -DENABLE_SUBINTERPRETERS=ON.
func TestHasSubinterpreterSupport(t *testing.T) {
	helpers.ResetMemoryStats()

	// Just verify the function is callable and returns a boolean.
	// We don't assert a specific value because this test runs in both
	// compile configurations.
	result := hasSubinterpreterSupport()
	t.Logf("has_subinterpreter_support() = %v", result)

	helpers.AssertMemoryUsage(t)
}

// TestSubinterpreterIsolation verifies that two check instances of the same
// check type have isolated module-level globals when sub-interpreters are
// enabled.
//
// isolation_check has a module-level `run_count` that increments on each run().
// With sub-interpreters (1:1 policy), each check gets its own interpreter and
// its own copy of the module, so both checks should return "1" after one run.
// Without sub-interpreters, they share the module, so the second returns "2".
func TestSubinterpreterIsolation(t *testing.T) {
	if !hasSubinterpreterSupport() {
		t.Skip("sub-interpreter support not compiled in (-DENABLE_SUBINTERPRETERS=ON required)")
	}

	helpers.ResetMemoryStats()

	res1, res2, err := runTwoIsolationChecks()
	if err != nil {
		t.Fatalf("runTwoIsolationChecks failed: %v", err)
	}

	// With sub-interpreters, each check has its own module-level run_count.
	// Both should return "1" (each incremented from 0 independently).
	if res1 != "1" {
		t.Errorf("check 1: expected run_count=\"1\", got \"%s\"", res1)
	}
	if res2 != "1" {
		t.Errorf("check 2: expected run_count=\"1\", got \"%s\" (module-level global leaked between sub-interpreters)", res2)
	}

	helpers.AssertMemoryUsage(t)
}
