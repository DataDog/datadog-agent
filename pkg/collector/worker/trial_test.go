// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"testing"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetTrialCallbacks clears the global callback slice for test isolation.
func resetTrialCallbacks(t *testing.T) {
	t.Helper()
	trialMu.Lock()
	defer trialMu.Unlock()
	trialResultCallbacks = nil
}

func TestRegisterAndNotifyTrialResult(t *testing.T) {
	resetTrialCallbacks(t)
	t.Cleanup(func() { resetTrialCallbacks(t) })

	type result struct {
		id checkid.ID
		ok bool
	}
	var got []result

	RegisterTrialResultCallback(func(id checkid.ID, ok bool) {
		got = append(got, result{id, ok})
	})

	notifyTrialResult("check:abc", true)
	notifyTrialResult("check:abc", false)

	require.Len(t, got, 2)
	assert.Equal(t, checkid.ID("check:abc"), got[0].id)
	assert.True(t, got[0].ok)
	assert.False(t, got[1].ok)
}

func TestNotifyTrialResultMultipleCallbacks(t *testing.T) {
	resetTrialCallbacks(t)
	t.Cleanup(func() { resetTrialCallbacks(t) })

	var calls1, calls2 int
	RegisterTrialResultCallback(func(_ checkid.ID, _ bool) { calls1++ })
	RegisterTrialResultCallback(func(_ checkid.ID, _ bool) { calls2++ })

	notifyTrialResult("check:x", true)

	assert.Equal(t, 1, calls1)
	assert.Equal(t, 1, calls2)
}
