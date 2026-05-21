// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/worker"
)

func TestTrialRegistry_FailuresUnderThresholdDoNotUnschedule(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	decision, shouldUnschedule := r.recordResult(id, false)
	require.Equal(t, worker.TrialResultContinue, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, false)
	require.Equal(t, worker.TrialResultContinue, decision)
	require.False(t, shouldUnschedule)
	// Third failure equals threshold — should signal unschedule.
	decision, shouldUnschedule = r.recordResult(id, false)
	require.Equal(t, worker.TrialResultRetire, decision)
	require.True(t, shouldUnschedule)
}

func TestTrialRegistry_SuccessResetsCounter(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	decision, shouldUnschedule := r.recordResult(id, false)
	require.Equal(t, worker.TrialResultContinue, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, false)
	require.Equal(t, worker.TrialResultContinue, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, true) // success resets
	require.Equal(t, worker.TrialResultPromote, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, false) // back to 1
	require.Equal(t, worker.TrialResultContinue, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, false) // 2
	require.Equal(t, worker.TrialResultContinue, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, false) // 3 = threshold
	require.Equal(t, worker.TrialResultRetire, decision)
	require.True(t, shouldUnschedule)
}

func TestTrialRegistry_RetireClearsCountAndSuppressesLateResults(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	r.recordResult(id, false)
	r.recordResult(id, false)
	r.retire(id)

	decision, shouldUnschedule := r.recordResult(id, true)
	require.Equal(t, worker.TrialResultRetire, decision)
	require.False(t, shouldUnschedule)
	decision, shouldUnschedule = r.recordResult(id, false)
	require.Equal(t, worker.TrialResultRetire, decision)
	require.False(t, shouldUnschedule)
}

func TestTrialRegistry_ResetClearsRetireState(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	r.retire(id)
	r.reset(id)

	decision, shouldUnschedule := r.recordResult(id, true)
	require.Equal(t, worker.TrialResultPromote, decision)
	require.False(t, shouldUnschedule)
}
