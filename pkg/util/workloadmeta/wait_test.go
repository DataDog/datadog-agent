// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// fakeStore reports itself as initialized after readyAfter calls to IsInitialized, or never if
// neverReady is set. WaitForInitialization polls from a single goroutine, so a plain counter is
// enough here.
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
	wmeta := &fakeStore{readyAfter: 3}

	assert.True(t, WaitForInitialization(wmeta, time.Minute, logmock.New(t)))
	assert.Equal(t, 4, wmeta.calls)
}

func TestWaitForInitializationTimeout(t *testing.T) {
	wmeta := &fakeStore{neverReady: true}
	timeout := 5 * pollInterval

	start := time.Now()
	assert.False(t, WaitForInitialization(wmeta, timeout, logmock.New(t)))

	// The wait is bounded: it gives up instead of blocking the command forever.
	assert.GreaterOrEqual(t, time.Since(start), timeout)
}

func TestWaitForInitializationDisabled(t *testing.T) {
	wmeta := &fakeStore{neverReady: true}

	assert.False(t, WaitForInitialization(wmeta, 0, logmock.New(t)))
	// A zero timeout still reports the current state, it just never polls again.
	assert.Equal(t, 1, wmeta.calls)
}
