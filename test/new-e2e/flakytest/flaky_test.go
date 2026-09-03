// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flakytest holds a single test that fails about half of the time it
// runs. It is only used by the new-e2e-orchestrion-flaky-test CI job to exercise
// orchestrion-instrumented `go test` runs (CI Visibility, flaky test detection).
package flakytest

import (
	"math/rand"
	"testing"
)

// TestRandomFailure fails approximately 50% of the time on purpose.
func TestRandomFailure(t *testing.T) {
	if rand.Intn(2) == 0 {
		t.Fatal("unlucky coin flip: this test fails about half of the time on purpose")
	}
}
