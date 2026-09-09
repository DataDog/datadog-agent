// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// fakeStore reports itself as initialized after readyAfter calls to IsInitialized, or never if
// neverReady is set.
type fakeStore struct {
	workloadmeta.Component

	readyAfter int
	neverReady bool
	calls      int
}

func (f *fakeStore) IsInitialized() bool {
	f.calls++
	return !f.neverReady && f.calls > f.readyAfter
}

func TestWaitForInitializationAlreadyInitialized(t *testing.T) {
	wmeta := &fakeStore{}

	assert.True(t, WaitForInitialization(wmeta, time.Minute, logmock.New(t)))
	// A single call means the ticker was never involved.
	assert.Equal(t, 1, wmeta.calls)
}

func TestWaitForInitializationWhilePolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		wmeta := &fakeStore{readyAfter: 3}

		assert.True(t, WaitForInitialization(wmeta, time.Minute, logmock.New(t)))
		// One poll per tick, returning on the first one that reports ready.
		assert.Equal(t, 4, wmeta.calls)
	})
}

func TestWaitForInitializationTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		wmeta := &fakeStore{neverReady: true}
		timeout := 5 * pollInterval
		start := time.Now()

		assert.False(t, WaitForInitialization(wmeta, timeout, logmock.New(t)))
		// It gives up on the deadline rather than polling forever.
		assert.Equal(t, timeout, time.Since(start))
	})
}

func TestWaitForInitializationDisabled(t *testing.T) {
	wmeta := &fakeStore{neverReady: true}

	assert.False(t, WaitForInitialization(wmeta, 0, logmock.New(t)))
	// Not even one probe: answering takes a lock that collector startup holds, so probing can
	// block, which a disabled wait must not do.
	assert.Zero(t, wmeta.calls)
}
