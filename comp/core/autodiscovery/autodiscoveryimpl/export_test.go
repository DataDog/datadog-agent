// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package autodiscoveryimpl

import "testing"

// Exported test-only aliases for use from the autodiscoveryimpl_test package.
// trial_worker_integration_test.go has to live in autodiscoveryimpl_test (not
// in autodiscoveryimpl) to import collectorimpl without creating a cycle:
// collectorimpl already imports autodiscoveryimpl via agent_check_metadata.go.

// TrialFailureThreshold mirrors trialFailureThreshold for cross-package tests.
const TrialFailureThreshold = trialFailureThreshold

// GetResolveTestSetup mirrors getResolveTestSetup for cross-package tests.
func GetResolveTestSetup(t *testing.T) (*MockScheduler, *AutoConfig, Deps) {
	return getResolveTestSetup(t)
}

// Schedules returns the Schedule-call count for cross-package tests.
func (ms *MockScheduler) Schedules() int64 { return ms.schedules.Load() }

// Unschedules returns the Unschedule-call count for cross-package tests.
func (ms *MockScheduler) Unschedules() int64 { return ms.unschedules.Load() }
