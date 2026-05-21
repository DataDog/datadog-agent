// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func TestTrialRegistry_FailuresUnderThresholdDoNotUnschedule(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	suppress, shouldUnschedule := r.recordResult(id, false)
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, false)
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	// Third failure equals threshold — should signal unschedule.
	suppress, shouldUnschedule = r.recordResult(id, false)
	require.True(t, suppress)
	require.True(t, shouldUnschedule)
}

func TestTrialRegistry_SuccessResetsCounter(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	suppress, shouldUnschedule := r.recordResult(id, false)
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, false)
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, true) // success resets
	require.False(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, false) // back to 1
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, false) // 2
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, false) // 3 = threshold
	require.True(t, suppress)
	require.True(t, shouldUnschedule)
}

func TestTrialRegistry_RetireClearsCountAndSuppressesLateResults(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	r.recordResult(id, false)
	r.recordResult(id, false)
	r.retire(id)

	suppress, shouldUnschedule := r.recordResult(id, true)
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
	suppress, shouldUnschedule = r.recordResult(id, false)
	require.True(t, suppress)
	require.False(t, shouldUnschedule)
}

func TestTrialRegistry_ResetClearsRetireState(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	r.retire(id)
	r.reset(id)

	suppress, shouldUnschedule := r.recordResult(id, true)
	require.False(t, suppress)
	require.False(t, shouldUnschedule)
}
