// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package admission

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

func resetGate(t *testing.T) {
	t.Helper()
	ResetGateForTesting()
	SetPaceForTesting(0, maxConfiguredDevices)
	SetFileStatsFuncForTesting(func() (*procfilestats.ProcessFileStats, error) {
		return &procfilestats.ProcessFileStats{
			AgentOpenFiles: 1,
			OsFileLimit:    1000,
		}, nil
	})
}

func TestDeviceCapExceeded(t *testing.T) {
	resetGate(t)

	g := Gate()
	for i := 0; i < maxConfiguredDevices; i++ {
		require.NoError(t, g.Admit(context.Background()))
	}

	err := g.Admit(context.Background())
	require.ErrorIs(t, err, ErrDeviceCapExceeded)
	require.Equal(t, maxConfiguredDevices, g.ConfiguredCount())
}

func TestReleaseDecrementsConfiguredCount(t *testing.T) {
	resetGate(t)

	g := Gate()
	require.NoError(t, g.Admit(context.Background()))
	require.Equal(t, 1, g.ConfiguredCount())

	g.Release()
	require.Equal(t, 0, g.ConfiguredCount())
}

func TestStartupPacingBlocksUntilTokensAvailable(t *testing.T) {
	resetGate(t)
	SetPaceForTesting(time.Hour, 1)

	g := Gate()
	ctx := context.Background()
	require.NoError(t, g.Admit(ctx))

	done := make(chan error, 1)
	go func() {
		done <- g.Admit(ctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("second admit should block, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	AddPacingTokensForTesting(1)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for paced admit")
	}
}

func TestFDGuardBlocksUntilUtilizationDrops(t *testing.T) {
	resetGate(t)
	SetFDRetryIntervalForTesting(20 * time.Millisecond)

	var agentOpenFiles atomic.Uint64
	agentOpenFiles.Store(900)
	SetFileStatsFuncForTesting(func() (*procfilestats.ProcessFileStats, error) {
		return &procfilestats.ProcessFileStats{
			AgentOpenFiles: agentOpenFiles.Load(),
			OsFileLimit:    1000,
		}, nil
	})

	g := Gate()
	done := make(chan error, 1)
	go func() {
		done <- g.Admit(context.Background())
	}()

	select {
	case err := <-done:
		t.Fatalf("admit should block on high FD utilization, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	agentOpenFiles.Store(100)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for FD guard to clear")
	}
}

func TestFDGuardSkipsOnErrNotImplemented(t *testing.T) {
	resetGate(t)
	SetFileStatsFuncForTesting(func() (*procfilestats.ProcessFileStats, error) {
		return nil, procfilestats.ErrNotImplemented
	})

	g := Gate()
	require.NoError(t, g.Admit(context.Background()))
}

func TestAdmitRespectsContextCancellation(t *testing.T) {
	resetGate(t)
	SetPaceForTesting(time.Hour, 0)
	SetFileStatsFuncForTesting(func() (*procfilestats.ProcessFileStats, error) {
		return &procfilestats.ProcessFileStats{
			AgentOpenFiles: 900,
			OsFileLimit:    1000,
		}, nil
	})

	g := Gate()
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		errCh <- g.Admit(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	wg.Wait()

	require.ErrorIs(t, <-errCh, context.Canceled)
}
