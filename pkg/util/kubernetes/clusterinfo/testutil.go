// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package clusterinfo

import (
	"context"
	"testing"
	"time"
)

// SetupStartupForTest resets the process-wide startup lookup and shortens its budgets for the
// duration of the test, so tests in other packages can drive the real startup path with a real
// Cluster Agent client. onEKS sets the local EKS evidence.
func SetupStartupForTest(t testing.TB, budget, later time.Duration, onEKS bool) {
	t.Helper()
	origBudget, origLater, origEvidence := startupRetryBudget, laterAttemptTimeout, hasEKSEvidence
	reset := func() {
		_ = startupIdentity.lock.Acquire(context.Background(), 1)
		defer startupIdentity.lock.Release(1)
		startupIdentity.tags, startupIdentity.budgetSpent = nil, false
		startupIdentity.lastErr, startupIdentity.lastErrAt = nil, time.Time{}
		identityCall.reset()
		clusterIDCall.reset()
	}
	reset()
	startupRetryBudget, laterAttemptTimeout = budget, later
	hasEKSEvidence = func() bool { return onEKS }
	t.Cleanup(func() {
		reset()
		startupRetryBudget, laterAttemptTimeout, hasEKSEvidence = origBudget, origLater, origEvidence
	})
}
