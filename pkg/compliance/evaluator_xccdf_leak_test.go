// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"slices"
	"testing"

	"go.uber.org/goleak"
)

// TestOSCAPIOEvaluateStopsGoroutines checks that an evaluation stops every
// goroutine it starts when oscap-io crashes, hangs and exits.
func TestOSCAPIOEvaluateStopsGoroutines(t *testing.T) {
	ignore := goleak.IgnoreCurrent()
	ids := []string{"a_pass", "b_crash", "c_hang", "d_fail"}
	got, err := evaluateFake(t.Context(), ids, 0, hangTimeout)
	if err != nil {
		t.Errorf("evaluate(%q) error = %v", ids, err)
	}
	if want := []string{"a_pass passed", "d_fail failed"}; !slices.Equal(got, want) {
		t.Errorf("evaluate(%q) = %q, want %q", ids, got, want)
	}
	goleak.VerifyNone(t, ignore)
}
