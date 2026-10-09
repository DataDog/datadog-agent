// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package admission

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

type notifyingClock struct {
	clock.Clock
	timers chan time.Duration
}

func (c *notifyingClock) Timer(d time.Duration) *clock.Timer {
	timer := c.Clock.Timer(d)
	c.timers <- d
	return timer
}

func newTestGate() (*AdmissionGate, *clock.Mock, <-chan time.Duration) {
	mockClock := clock.NewMock()
	clk := &notifyingClock{
		Clock:  mockClock,
		timers: make(chan time.Duration, 1),
	}
	g := newGate(clk)
	g.paceInterval = 0
	g.paceBurst = maxConfiguredDevices
	g.tokens = maxConfiguredDevices
	g.getFileStats = func() (*procfilestats.ProcessFileStats, error) {
		return &procfilestats.ProcessFileStats{
			AgentOpenFiles: 1,
			OsFileLimit:    1000,
		}, nil
	}
	return g, mockClock, clk.timers
}

func waitForTimer(t *testing.T, timers <-chan time.Duration) time.Duration {
	t.Helper()
	select {
	case d := <-timers:
		return d
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for admission retry timer")
		return 0
	}
}

func waitForAdmit(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for admission")
		return nil
	}
}

func TestDeviceCapExceeded(t *testing.T) {
	g, _, _ := newTestGate()
	for i := 0; i < maxConfiguredDevices; i++ {
		require.NoError(t, g.Admit(context.Background()))
	}

	err := g.Admit(context.Background())
	require.ErrorIs(t, err, ErrDeviceCapExceeded)
	require.Equal(t, maxConfiguredDevices, g.ConfiguredCount())
}

func TestReleaseDecrementsConfiguredCount(t *testing.T) {
	g, _, _ := newTestGate()
	require.NoError(t, g.Admit(context.Background()))
	require.Equal(t, 1, g.ConfiguredCount())

	g.Release()
	require.Equal(t, 0, g.ConfiguredCount())
}

func TestStartupPacingBlocksUntilTokensAvailable(t *testing.T) {
	g, mockClock, timers := newTestGate()
	g.paceInterval = time.Hour
	g.paceBurst = 1
	g.tokens = 1

	require.NoError(t, g.Admit(context.Background()))

	done := make(chan error, 1)
	go func() {
		done <- g.Admit(context.Background())
	}()

	require.Equal(t, time.Hour, waitForTimer(t, timers))
	require.Equal(t, 1, g.ConfiguredCount())

	mockClock.Add(time.Hour)
	require.NoError(t, waitForAdmit(t, done))
	require.Equal(t, 2, g.ConfiguredCount())
}

func TestFDGuardBlocksUntilUtilizationDrops(t *testing.T) {
	g, mockClock, timers := newTestGate()
	g.fdRetryInterval = 20 * time.Millisecond

	var agentOpenFiles atomic.Uint64
	agentOpenFiles.Store(900)
	g.getFileStats = func() (*procfilestats.ProcessFileStats, error) {
		return &procfilestats.ProcessFileStats{
			AgentOpenFiles: agentOpenFiles.Load(),
			OsFileLimit:    1000,
		}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- g.Admit(context.Background())
	}()

	require.Equal(t, 20*time.Millisecond, waitForTimer(t, timers))
	require.Equal(t, 0, g.ConfiguredCount())

	agentOpenFiles.Store(100)
	mockClock.Add(20 * time.Millisecond)
	require.NoError(t, waitForAdmit(t, done))
	require.Equal(t, 1, g.ConfiguredCount())
}

func TestFDGuardSkipsOnErrNotImplemented(t *testing.T) {
	g, _, _ := newTestGate()
	g.getFileStats = func() (*procfilestats.ProcessFileStats, error) {
		return nil, procfilestats.ErrNotImplemented
	}

	require.NoError(t, g.Admit(context.Background()))
}

func TestAdmitRespectsContextCancellation(t *testing.T) {
	g, _, timers := newTestGate()
	g.fdRetryInterval = time.Hour
	g.getFileStats = func() (*procfilestats.ProcessFileStats, error) {
		return &procfilestats.ProcessFileStats{
			AgentOpenFiles: 900,
			OsFileLimit:    1000,
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- g.Admit(ctx)
	}()

	require.Equal(t, time.Hour, waitForTimer(t, timers))
	cancel()

	require.ErrorIs(t, waitForAdmit(t, done), context.Canceled)
	require.Equal(t, 0, g.ConfiguredCount())
}
