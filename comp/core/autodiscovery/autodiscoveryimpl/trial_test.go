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

	require.False(t, r.recordResult(id, false))
	require.False(t, r.recordResult(id, false))
	// Third failure equals threshold — should signal unschedule.
	require.True(t, r.recordResult(id, false))
}

func TestTrialRegistry_SuccessResetsCounter(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")

	require.False(t, r.recordResult(id, false))
	require.False(t, r.recordResult(id, false))
	require.False(t, r.recordResult(id, true))  // success resets
	require.False(t, r.recordResult(id, false)) // back to 1
	require.False(t, r.recordResult(id, false)) // 2
	require.True(t, r.recordResult(id, false))  // 3 = threshold
}

func TestTrialRegistry_ForgetClears(t *testing.T) {
	r := newTrialRegistry(3)
	id := checkid.ID("krakend:abc")
	r.recordResult(id, false)
	r.recordResult(id, false)
	r.forget(id)
	require.False(t, r.recordResult(id, false)) // counter restarted
}
