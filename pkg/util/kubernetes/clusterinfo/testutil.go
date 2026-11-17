// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package clusterinfo

import (
	"testing"
	"time"
)

// SetupStartupForTest resets the process-wide startup fetches and shortens the startup budget for
// the duration of the test, so tests in other packages can drive the real startup path with a
// real Cluster Agent client. onEKS sets the local EKS evidence.
func SetupStartupForTest(t testing.TB, budget time.Duration, onEKS bool) {
	t.Helper()
	origBudget, origEvidence := startupRetryBudget, hasEKSEvidence
	reset := func() {
		startupFetch.mu.Lock()
		startupFetch.f = nil
		startupFetch.mu.Unlock()
		clusterIDCall.reset()
	}
	reset()
	startupRetryBudget = budget
	hasEKSEvidence = func() bool { return onEKS }
	t.Cleanup(func() {
		reset()
		startupRetryBudget, hasEKSEvidence = origBudget, origEvidence
	})
}
