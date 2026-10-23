// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// liveLaunchd is a launchctl stand-in that tracks which labels are loaded: bootout unloads a
// label, kickstart loads it, and print reports a loaded label as running under this test's own
// pid, which the exit observer can arm on and which never exits during the test.
type liveLaunchd struct {
	mu     sync.Mutex
	loaded map[string]bool
	calls  [][]string
	// failPrint makes print fail, once, for the targets it names.
	failPrint map[string]bool
}

func stubLiveLaunchd(t *testing.T, loaded ...string) *liveLaunchd {
	t.Helper()

	l := &liveLaunchd{loaded: map[string]bool{}, failPrint: map[string]bool{}}
	for _, label := range loaded {
		l.loaded["system/"+label] = true
	}
	original := launchdClient
	launchdClient = func() *launchd.Client {
		client := launchd.NewClient(launchd.System)
		client.BootoutSettlePollInterval = time.Millisecond
		client.Runner = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.calls = append(l.calls, args)
			target := args[len(args)-1]
			switch args[0] {
			case "print":
				if l.failPrint[target] {
					delete(l.failPrint, target)
					return nil, errors.New("injected failure")
				}
				if l.loaded[target] {
					return []byte(fmt.Sprintf("%s = {\n\tpid = %d\n}\n", target, os.Getpid())), nil
				}
				return []byte(notLoadedOutput), exitError(113)
			case "bootout":
				l.loaded[target] = false
			case "kickstart":
				l.loaded[target] = true
			}
			return nil, nil
		}
		return client
	}
	t.Cleanup(func() { launchdClient = original })
	return l
}

func (l *liveLaunchd) isLoaded(label string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded["system/"+label]
}

func shortFailSafeBackoff(t *testing.T) {
	t.Helper()

	original := watcherFailSafeBackoff
	watcherFailSafeBackoff = time.Millisecond
	t.Cleanup(func() { watcherFailSafeBackoff = original })
}

// runningExperiment lays out a host mid-experiment: the experiment configuration deployed, the
// deadline written, the experiment agent jobs and the stable daemon loaded.
func runningExperiment(t *testing.T) (*liveLaunchd, string, string) {
	t.Helper()

	running := []string{installerJob}
	for _, label := range agentJobs {
		running = append(running, label+"-exp")
	}
	l := stubLiveLaunchd(t, running...)
	stubJobDir(t)
	deadlinePath := stubDeadlinePath(t)
	stubConfigExperimentLock(t)
	shortRestoreBackoff(t)
	shortFailSafeBackoff(t)
	dirs := stubConfigExperimentDirs(t)
	deployExperimentConfig(t, dirs)
	require.NoError(t, experimentDeadline().Write(time.Hour))
	return l, deadlinePath, dirs.ExperimentPath
}

// assertReverted checks the host is back on the stable job set, with the experiment configuration
// discarded and no deadline left.
func assertReverted(t *testing.T, l *liveLaunchd, experimentPath string) {
	t.Helper()

	for _, label := range configExperimentJobs {
		assert.True(t, l.isLoaded(label), "%s should be back on its stable job", label)
		assert.False(t, l.isLoaded(label+"-exp"), "%s should be unloaded", label+"-exp")
	}
	info, err := os.Lstat(experimentPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the experiment configuration should be discarded")
	_, err = os.Lstat(configExperimentDeadlinePath)
	assert.True(t, os.IsNotExist(err), "no deadline should be left")
}

// TestWatcherRevertsWhenItCannotResolveTheExperimentJobs pins the fail-safe: a launchctl failure
// that keeps the watcher from finding the experiment's processes reverts the experiment instead of
// leaving it running with nothing watching it.
func TestWatcherRevertsWhenItCannotResolveTheExperimentJobs(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	l.failPrint["system/"+agentJob+"-exp"] = true

	err := watchExperiment(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experiment reverted")
	assertReverted(t, l, experimentPath)
}

// TestWatcherRevertsWhenItCanNoLongerReadTheDeadline pins the fail-safe on the deadline tick: a
// deadline that keeps failing to read -- here, because its directory stopped being trustworthy --
// reverts the experiment rather than letting it run past a bound nobody can check.
func TestWatcherRevertsWhenItCanNoLongerReadTheDeadline(t *testing.T) {
	original := deadlineTickInterval
	deadlineTickInterval = 5 * time.Millisecond
	t.Cleanup(func() { deadlineTickInterval = original })
	l, deadlinePath, experimentPath := runningExperiment(t)

	done := make(chan error, 1)
	go func() { done <- watchExperiment(context.Background()) }()
	require.Eventually(t, func() bool { return l.isLoaded(installerJob + "-exp") }, 5*time.Second, time.Millisecond,
		"the watcher should hand the daemon over before supervising")
	require.NoError(t, os.Chmod(filepath.Dir(deadlinePath), 0777))
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(deadlinePath), 0755) })

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "experiment reverted")
	case <-time.After(10 * time.Second):
		t.Fatal("the watcher should have given up on the deadline and reverted")
	}
	assertReverted(t, l, experimentPath)
}

// TestFailSafeRevertLeavesAFinishedExperimentAlone pins that the fail-safe does not revert an
// experiment that was stopped, promoted or replaced in the meantime.
func TestFailSafeRevertLeavesAFinishedExperimentAlone(t *testing.T) {
	l, _, _ := runningExperiment(t)
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	require.NoError(t, experimentDeadline().Write(time.Hour))

	require.NoError(t, failSafeRevert(context.Background(), experimentDeadline(), token, errors.New("injected")))
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label+"-exp"), "a newer experiment must be left running")
	}
}

// TestFailSafeRevertRevertsWithoutAnUnusableLock pins the last resort: when the installer lock
// itself cannot be taken, no installer command can run either, so the experiment is reverted
// without it rather than left unsupervised.
func TestFailSafeRevertRevertsWithoutAnUnusableLock(t *testing.T) {
	l, _, experimentPath := runningExperiment(t)
	// A directory where the lock database should be: bbolt cannot open it.
	require.NoError(t, os.Mkdir(configExperimentLockPath, 0755))
	token, err := experimentDeadline().Read()
	require.NoError(t, err)

	err = failSafeRevert(context.Background(), experimentDeadline(), token, errors.New("injected"))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "experiment reverted"), err.Error())
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label), "%s should be back on its stable job", label)
	}
	info, err := os.Lstat(experimentPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}

// TestFailSafeRevertDoesNothingOnceStopped pins that a watcher stopped on purpose -- the resume
// step replacing it -- reverts nothing.
func TestFailSafeRevertDoesNothingOnceStopped(t *testing.T) {
	l, _, _ := runningExperiment(t)
	token, err := experimentDeadline().Read()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.Error(t, failSafeRevert(ctx, experimentDeadline(), token, errors.New("injected")))
	for _, label := range agentJobs {
		assert.True(t, l.isLoaded(label+"-exp"))
	}
}
