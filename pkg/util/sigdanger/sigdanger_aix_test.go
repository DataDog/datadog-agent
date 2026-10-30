// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package sigdanger

import (
	"runtime/debug"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRateLimit(t *testing.T) {
	base := time.Now()
	current := base
	now = func() time.Time { return current }
	defer func() { now = time.Now }()

	called := make(chan struct{}, 10)
	freeOSMemory = func() { called <- struct{}{} }
	defer func() { freeOSMemory = debug.FreeOSMemory }()

	mu.Lock()
	lastHandled = base.Add(-time.Hour)
	mu.Unlock()

	handle()
	require.Len(t, called, 1, "first SIGDANGER should be handled")

	// Immediately after: rate-limited.
	handle()
	assert.Len(t, called, 1, "SIGDANGER within handleInterval should be rate-limited")

	// After the interval: handled again.
	current = current.Add(2 * handleInterval)
	handle()
	require.Len(t, called, 2, "SIGDANGER after handleInterval should be handled")
}

func TestInitCatchesDeliveredSIGDANGER(t *testing.T) {
	called := make(chan struct{}, 1)
	freeOSMemory = func() { called <- struct{}{} }
	defer func() { freeOSMemory = debug.FreeOSMemory }()

	mu.Lock()
	lastHandled = time.Now().Add(-time.Hour)
	mu.Unlock()

	Init()

	require.NoError(t, unix.Kill(unix.Getpid(), unix.SIGDANGER))

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("SIGDANGER was not handled within 5s")
	}
}
