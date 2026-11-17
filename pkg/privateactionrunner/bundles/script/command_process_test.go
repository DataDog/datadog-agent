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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/scriptprocess"
)

// The subprocesses deliberately inherit the output handles, just like a script
// starting background work; "detach" mode gives the child its own null handles.
// Use the test executable so this works on Windows too.
func TestScriptProcess(t *testing.T) {
	i := slices.Index(os.Args, "--")
	if i < 0 || i+1 == len(os.Args) {
		return
	}
	args := os.Args[i+1:]
	if args[0] == "success" {
		os.Exit(0)
	}
	if args[0] == "failure" {
		os.Exit(7)
	}
	if args[0] == "child" {
		time.Sleep(60 * time.Second)
		os.Exit(0)
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	child := exec.Command(exe, "-test.run=^TestScriptProcess$", "--", "child")
	if args[0] != "detach" {
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
	}
	require.NoError(t, child.Start())
	// On Windows the script is the PowerShell wrapper that launched this process.
	scriptPid := os.Getpid()
	if runtime.GOOS == "windows" {
		scriptPid = os.Getppid()
	}
	require.NoError(t, os.WriteFile(scriptPidFile(args[1]), []byte(strconv.Itoa(scriptPid)), 0600))
	require.NoError(t, os.WriteFile(args[1], []byte(strconv.Itoa(child.Process.Pid)), 0600))
	if args[0] == "wait" {
		_ = child.Wait()
	}
	os.Exit(0)
}

func TestScriptCommandInheritedPipes(t *testing.T) {
	for _, mode := range []string{"exit", "wait"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			exe, err := os.Executable()
			require.NoError(t, err)
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			cmd := processTestCommand(t, ctx, exe, []string{"-test.run=^TestScriptProcess$", "--", mode, pidFile})
			done := make(chan error, 1)
			go func() {
				_, err := scriptprocess.Run(ctx, cmd, defaultMaxOutputSize)
				done <- err
			}()
			child := waitForTestChild(t, pidFile)
			if mode == "wait" {
				cancel()
			}
			start := time.Now()
			select {
			case err := <-done:
				if mode == "exit" {
					require.ErrorIs(t, err, exec.ErrWaitDelay)
				} else {
					require.Error(t, err)
				}
				if runtime.GOOS == "linux" {
					// Descendants must not outlive the script, whether it exited or was canceled.
					require.Eventually(t, func() bool { return processStopped(child.Pid) },
						2*time.Second, 10*time.Millisecond, "child survived the script")
				}
				t.Logf("%s released output pipes after %s: %v", mode, time.Since(start), err)
			case <-time.After(10 * time.Second):
				_ = child.Kill()
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
				}
				t.Fatal("script retained its output pipes for more than 10 seconds")
			}
		})
	}
}

func scriptPidFile(pidFile string) string { return pidFile + ".script" }

func waitForTestChild(t *testing.T, pidFile string) *os.Process {
	t.Helper()
	var child *os.Process
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return false
		}
		child, err = os.FindProcess(pid)
		return err == nil
	}, 15*time.Second, 10*time.Millisecond, "child did not start")
	t.Cleanup(func() { _ = child.Kill(); _ = child.Release() })
	return child
}
