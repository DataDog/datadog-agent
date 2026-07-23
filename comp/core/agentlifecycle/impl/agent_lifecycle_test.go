// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package agentlifecycleimpl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/require"

	agentlifecycle "github.com/DataDog/datadog-agent/comp/core/agentlifecycle/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

type fakeLocker struct {
	attempted chan struct{}
	allow     chan struct{}
	unlocks   atomic.Int32
}

func newFakeLocker() *fakeLocker {
	return &fakeLocker{attempted: make(chan struct{}), allow: make(chan struct{})}
}

func (l *fakeLocker) TryLockContext(ctx context.Context, _ time.Duration) (bool, error) {
	close(l.attempted)
	select {
	case <-l.allow:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (l *fakeLocker) Unlock() error {
	l.unlocks.Add(1)
	return nil
}

func TestDisabledLifecycleIsNoop(t *testing.T) {
	deps := dependencies{Config: config.NewMock(t), Log: logmock.New(t)}
	comp, err := newComponent(deps, func(string) fileLocker {
		t.Fatal("disabled lifecycle must not create a lock")
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, comp.Wait(context.Background()))
	require.NoError(t, comp.MarkActive())
	require.NoError(t, comp.Close())
}

func TestLifecycleStateTransitions(t *testing.T) {
	comp, locker, statePath := newEnabledComponent(t)

	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(context.Background()) }()

	<-locker.attempted
	require.Equal(t, agentlifecycle.StatePrepared, readState(t, statePath))
	close(locker.allow)
	require.NoError(t, <-waitResult)
	require.Equal(t, agentlifecycle.StateActivating, readState(t, statePath))

	require.NoError(t, comp.MarkActive())
	require.Equal(t, agentlifecycle.StateActive, readState(t, statePath))

	require.NoError(t, comp.Close())
	require.Equal(t, agentlifecycle.StateStopped, readState(t, statePath))
	require.EqualValues(t, 1, locker.unlocks.Load())
	require.NoError(t, comp.Close(), "Close must be idempotent")
	require.EqualValues(t, 1, locker.unlocks.Load())
}

func TestLifecycleWaitCancellation(t *testing.T) {
	comp, locker, statePath := newEnabledComponent(t)
	ctx, cancel := context.WithCancel(context.Background())
	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(ctx) }()

	<-locker.attempted
	require.Equal(t, agentlifecycle.StatePrepared, readState(t, statePath))
	cancel()
	require.ErrorIs(t, <-waitResult, context.Canceled)
	require.NoError(t, comp.Close())
	require.Zero(t, locker.unlocks.Load())
}

func TestRealFileLockHandsOwnershipToPreparedReplacement(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "locks", "{component}.lock")
	newProcess := func(componentName string) agentlifecycle.Component {
		t.Helper()
		deps := dependencies{
			Config: config.NewMockWithOverrides(t, map[string]interface{}{
				rolloutEnabledKey:   true,
				rolloutLockPathKey:  lockPath,
				rolloutStatePathKey: filepath.Join(dir, "state", "{component}.state"),
			}),
			Log:    logmock.New(t),
			Params: agentlifecycle.Params{ComponentName: componentName},
		}
		comp, err := newComponentForPlatform(deps, func(path string) fileLocker { return flock.New(path) }, "linux")
		require.NoError(t, err)
		return comp
	}

	oldProcess := newProcess("agent")
	replacement := newProcess("agent")
	require.NoError(t, oldProcess.Wait(context.Background()))

	replacementResult := make(chan error, 1)
	go func() { replacementResult <- replacement.Wait(context.Background()) }()
	require.Eventually(t, func() bool {
		contents, err := os.ReadFile(filepath.Join(dir, "state", "agent.state"))
		return err == nil && strings.TrimSpace(string(contents)) == agentlifecycle.StatePrepared
	}, time.Second, 10*time.Millisecond)
	select {
	case err := <-replacementResult:
		t.Fatalf("replacement acquired ownership before the old process stopped: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	require.NoError(t, oldProcess.Close())
	select {
	case err := <-replacementResult:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("replacement did not acquire ownership after the old process stopped")
	}
	require.Equal(t, agentlifecycle.StateActivating, readState(t, filepath.Join(dir, "state", "agent.state")))
	require.NoError(t, replacement.Close())
}

func TestLifecycleRequiresValidPaths(t *testing.T) {
	tests := map[string]map[string]interface{}{
		"relative lock": {
			rolloutLockPathKey:  "test-agent.lock",
			rolloutStatePathKey: filepath.Join(t.TempDir(), "test-agent.state"),
		},
		"relative state": {
			rolloutLockPathKey:  filepath.Join(t.TempDir(), "test-agent.lock"),
			rolloutStatePathKey: "test-agent.state",
		},
		"same path": {
			rolloutLockPathKey:  filepath.Join(t.TempDir(), "test-agent.lock"),
			rolloutStatePathKey: "",
		},
		"shared lock path": {
			rolloutLockPathKey:  filepath.Join(t.TempDir(), "agent.lock"),
			rolloutStatePathKey: filepath.Join(t.TempDir(), "test-agent.state"),
		},
		"shared state path": {
			rolloutLockPathKey:  filepath.Join(t.TempDir(), "test-agent.lock"),
			rolloutStatePathKey: filepath.Join(t.TempDir(), "agent.state"),
		},
	}
	for name, overrides := range tests {
		t.Run(name, func(t *testing.T) {
			overrides[rolloutEnabledKey] = true
			if name == "same path" {
				overrides[rolloutStatePathKey] = overrides[rolloutLockPathKey]
			}
			deps := dependencies{
				Config: config.NewMockWithOverrides(t, overrides),
				Log:    logmock.New(t),
				Params: agentlifecycle.Params{ComponentName: "test-agent"},
			}
			_, err := newComponentForPlatform(deps, func(string) fileLocker { return newFakeLocker() }, "linux")
			require.Error(t, err)
		})
	}
}

func TestComponentPathResolution(t *testing.T) {
	tests := map[string]struct {
		configured string
		component  string
		suffix     string
		expected   string
	}{
		"template": {
			configured: "/var/run/datadog/{component}.lock",
			component:  "core-agent",
			suffix:     ".lock",
			expected:   "/var/run/datadog/core-agent.lock",
		},
		"operator expanded path": {
			configured: "/var/run/datadog/trace-agent.state",
			component:  "trace-agent",
			suffix:     ".state",
			expected:   "/var/run/datadog/trace-agent.state",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resolved, err := resolveComponentPath(test.configured, test.component, test.suffix, "test.path")
			require.NoError(t, err)
			require.Equal(t, test.expected, resolved)
		})
	}
}

func TestComponentPathRejectsSharedLiteral(t *testing.T) {
	_, err := resolveComponentPath("/var/run/datadog/agent.lock", "core-agent", ".lock", "test.path")
	require.ErrorContains(t, err, "must contain {component}")
}

func TestPreparedRolloutRejectsWindows(t *testing.T) {
	require.ErrorContains(t, validatePlatform("windows"), "Linux-only")
	require.ErrorContains(t, validatePlatform("darwin"), "Linux-only")
	require.NoError(t, validatePlatform("linux"))
}

func TestComponentPathRejectsTraversalName(t *testing.T) {
	_, err := resolveComponentPath("/var/run/datadog/{component}.lock", "..", ".lock", "test.path")
	require.ErrorContains(t, err, "path-safe")
}

func TestMarkActiveRequiresOwnership(t *testing.T) {
	comp, _, _ := newEnabledComponent(t)
	require.Error(t, comp.MarkActive())
}

func TestWaitPropagatesLockerError(t *testing.T) {
	comp, locker, _ := newEnabledComponent(t)
	expected := errors.New("lock failed")
	locker.allow = nil
	comp.(*component).locker = errorLocker{err: expected}
	require.ErrorIs(t, comp.Wait(context.Background()), expected)
}

type errorLocker struct{ err error }

func (l errorLocker) TryLockContext(context.Context, time.Duration) (bool, error) {
	return false, l.err
}
func (errorLocker) Unlock() error { return nil }

func newEnabledComponent(t *testing.T) (agentlifecycle.Component, *fakeLocker, string) {
	t.Helper()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state", "test-agent.state")
	locker := newFakeLocker()
	deps := dependencies{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			rolloutEnabledKey:   true,
			rolloutLockPathKey:  filepath.Join(dir, "lock", "test-agent.lock"),
			rolloutStatePathKey: statePath,
		}),
		Log:    logmock.New(t),
		Params: agentlifecycle.Params{ComponentName: "test-agent"},
	}
	comp, err := newComponentForPlatform(deps, func(string) fileLocker { return locker }, "linux")
	require.NoError(t, err)
	return comp, locker, statePath
}

func readState(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.TrimSpace(string(contents))
}
