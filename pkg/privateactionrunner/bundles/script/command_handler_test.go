// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_script

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func runProcessTestHandler(t *testing.T, ctx context.Context, config ScriptBundleConfig, noFail bool, timeout int) (any, error) {
	t.Helper()
	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	credentials := &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{
		{Name: "configFileLocation", Value: string(data)},
	}}
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: map[string]interface{}{
		"scriptName": "test", "noFailOnError": noFail, "timeout": timeout,
	}}
	if runtime.GOOS == "windows" {
		return NewRunPredefinedPowershellScriptHandler().Run(ctx, task, credentials)
	}
	return NewRunPredefinedScriptHandler().Run(ctx, task, credentials)
}

func TestScriptHandlerExecutionErrors(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name, mode                     string
		noFail, startFailure, canceled bool
		timeout                        int
		wantError                      string
	}{
		{name: "success", mode: "success"},
		{name: "nonzero exit", mode: "failure", wantError: "failed to execute"},
		{name: "allowed nonzero exit", mode: "failure", noFail: true},
		{name: "start failure", mode: "success", startFailure: true, wantError: "failed to execute"},
		{name: "canceled", mode: "success", canceled: true, wantError: "context canceled"},
		{name: "timeout", mode: "child", timeout: 1, wantError: "timed out after 1 seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			command := exe
			if tc.startFailure {
				if runtime.GOOS == "windows" {
					// Prevent powershell.exe itself from starting, rather than failing inside a script.
					t.Setenv("PATH", t.TempDir())
				} else {
					command = filepath.Join(t.TempDir(), "missing-command")
				}
			}
			config := processTestConfig(command, []string{"-test.run=^TestScriptProcess$", "--", tc.mode})
			if tc.timeout > 0 {
				config = timeoutTestConfig()
			}
			output, err := runProcessTestHandler(t, ctx, config, tc.noFail, tc.timeout)
			if tc.wantError == "" {
				require.NoError(t, err)
				require.NotNil(t, output)
			} else {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, output)
			}
		})
	}
}

func TestScriptHandlerReleasesInheritedPipes(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runProcessTestHandler(t, ctx, processTestConfig(exe, []string{"-test.run=^TestScriptProcess$", "--", "exit", pidFile}), false, 0)
		done <- err
	}()
	child := waitForTestChild(t, pidFile)
	select {
	case err := <-done:
		require.ErrorIs(t, err, exec.ErrWaitDelay)
	case <-time.After(10 * time.Second):
		_ = child.Kill()
		cancel()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
		}
		t.Fatal("handler retained its output pipes for more than 10 seconds")
	}
}

func TestScriptHandlerReportsTimeoutAfterScriptExit(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	// Fire the deadline only once the child runs, so slow script startup cannot race it.
	ctx := newManualDeadlineContext()
	done := make(chan error, 1)
	go func() {
		// The script exits at once while its child holds the output pipes until the deadline.
		_, err := runProcessTestHandler(t, ctx, processTestConfig(exe, []string{"-test.run=^TestScriptProcess$", "--", "exit", pidFile}), false, 60)
		done <- err
	}()
	child := waitForTestChild(t, pidFile)
	// Expiring while the script still runs would kill it and pass without the after-exit path.
	data, err := os.ReadFile(scriptPidFile(pidFile))
	require.NoError(t, err)
	scriptPid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	require.Eventually(t, func() bool { return processStopped(scriptPid) },
		15*time.Second, 10*time.Millisecond, "script did not exit")
	ctx.expire()
	fired := time.Now()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "timed out after 60 seconds")
		// Stay under the 5s pipe wait delay so the deadline, not the wait delay, ended the run.
		require.Less(t, time.Since(fired), 4*time.Second)
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not return after the deadline")
	}
	require.Eventually(t, func() bool { return processStopped(child.Pid) },
		2*time.Second, 10*time.Millisecond, "child survived the timeout")
}

// manualDeadlineContext reports context.DeadlineExceeded once expire is called.
type manualDeadlineContext struct {
	context.Context
	done chan struct{}
	once sync.Once
}

func newManualDeadlineContext() *manualDeadlineContext {
	return &manualDeadlineContext{Context: context.Background(), done: make(chan struct{})}
}

func (c *manualDeadlineContext) expire() { c.once.Do(func() { close(c.done) }) }

func (c *manualDeadlineContext) Done() <-chan struct{} { return c.done }

func (c *manualDeadlineContext) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestScriptHandlerTerminatesDetachedDescendants(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	_, err = runProcessTestHandler(t, context.Background(), processTestConfig(exe, []string{"-test.run=^TestScriptProcess$", "--", "detach", pidFile}), false, 0)
	require.NoError(t, err)
	child := waitForTestChild(t, pidFile)
	require.Eventually(t, func() bool { return processStopped(child.Pid) },
		2*time.Second, 10*time.Millisecond, "detached child survived the script")
}
