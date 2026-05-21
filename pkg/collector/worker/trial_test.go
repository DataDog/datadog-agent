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

// resetTrialCallback clears the global callback for test isolation.
func resetTrialCallback(t *testing.T) {
	t.Helper()
	trialMu.Lock()
	defer trialMu.Unlock()
	trialResultCallback = nil
}

func TestRegisterAndNotifyTrialResult(t *testing.T) {
	resetTrialCallback(t)
	t.Cleanup(func() { resetTrialCallback(t) })

	type result struct {
		id checkid.ID
		ok bool
	}
	var got []result

	RegisterTrialResultCallback(func(id checkid.ID, ok bool) bool {
		got = append(got, result{id, ok})
		return !ok
	})

	assert.False(t, notifyTrialResult("check:abc", true))
	assert.True(t, notifyTrialResult("check:abc", false))

	require.Len(t, got, 2)
	assert.Equal(t, checkid.ID("check:abc"), got[0].id)
	assert.True(t, got[0].ok)
	assert.False(t, got[1].ok)
}

func TestRegisterTrialResultCallbackReplacesPreviousCallback(t *testing.T) {
	resetTrialCallback(t)
	t.Cleanup(func() { resetTrialCallback(t) })

	var calls int
	RegisterTrialResultCallback(func(_ checkid.ID, _ bool) bool {
		t.Fatal("replaced callback should not be called")
		return true
	})
	RegisterTrialResultCallback(func(_ checkid.ID, _ bool) bool {
		calls++
		return true
	})

	assert.True(t, notifyTrialResult("check:x", true))
	assert.Equal(t, 1, calls)
}

func TestNotifyTrialResultDefaults(t *testing.T) {
	resetTrialCallback(t)
	t.Cleanup(func() { resetTrialCallback(t) })

	assert.False(t, notifyTrialResult("check:x", true))
	assert.True(t, notifyTrialResult("check:x", false))
}
