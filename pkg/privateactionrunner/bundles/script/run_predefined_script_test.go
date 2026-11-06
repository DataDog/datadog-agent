// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const testScriptName = "test"

func runPredefinedScript(t *testing.T, script RunPredefinedScriptConfig, inputs map[string]interface{}) (*RunPredefinedScriptOutputs, error) {
	t.Helper()

	config, err := yaml.Marshal(ScriptBundleConfig{
		SchemaId:            schemaIdV1,
		RunPredefinedScript: map[string]RunPredefinedScriptConfig{testScriptName: script},
	})
	require.NoError(t, err)
	credentials := &privateconnection.PrivateCredentials{
		Tokens: []privateconnection.PrivateCredentialsToken{{Name: "configFileLocation", Value: string(config)}},
	}

	inputs["scriptName"] = testScriptName
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: inputs}

	out, err := NewRunPredefinedScriptHandler().Run(context.Background(), task, credentials)
	if err != nil {
		return nil, err
	}
	return out.(*RunPredefinedScriptOutputs), nil
}

func requireProcessTerminated(t *testing.T, pid int) {
	t.Helper()
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}, 2*time.Second, 10*time.Millisecond, "descendant process %d was not terminated", pid)
}

func TestRunPredefinedScript_Success(t *testing.T) {
	out, err := runPredefinedScript(t, RunPredefinedScriptConfig{
		Command: []string{"/bin/sh", "-c", "echo hello; echo world 1>&2"},
	}, map[string]interface{}{})

	require.NoError(t, err)
	assert.Equal(t, 0, out.ExitCode)
	assert.Equal(t, "hello", out.Stdout)
	assert.Equal(t, "world", out.Stderr)
}

func TestRunPredefinedScript_TimeoutKillsDescendantsHoldingStdio(t *testing.T) {
	if runtime.GOOS == "aix" {
		t.Skip("process-group assertion is not supported on AIX")
	}

	for name, script := range map[string]string{
		"script still running":  `sleep 30 & echo $! > "$CHILD_PID_FILE"; wait`,
		"script already exited": `sleep 30 & echo $! > "$CHILD_PID_FILE"`,
	} {
		t.Run(name, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			t.Setenv("CHILD_PID_FILE", pidFile)

			start := time.Now()
			_, err := runPredefinedScript(t, RunPredefinedScriptConfig{
				Command:        []string{"/bin/sh", "-c", script},
				AllowedEnvVars: []string{"CHILD_PID_FILE"},
			}, map[string]interface{}{"timeout": 1})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "timed out")
			// Stay under the 5s pipe grace period so the deadline, not the grace period, ended the run.
			assert.Less(t, time.Since(start), 4*time.Second, "script outlived its timeout")

			pidBytes, err := os.ReadFile(pidFile)
			require.NoError(t, err)
			childPID, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
			require.NoError(t, err)
			requireProcessTerminated(t, childPID)
		})
	}
}

func TestRunPredefinedScript_ReapsDetachedDescendants(t *testing.T) {
	if runtime.GOOS == "aix" {
		t.Skip("process-group assertion is not supported on AIX")
	}

	out, err := runPredefinedScript(t, RunPredefinedScriptConfig{
		Command: []string{"/bin/sh", "-c", "sleep 30 </dev/null >/dev/null 2>&1 & echo $!"},
	}, map[string]interface{}{})

	require.NoError(t, err)
	assert.Equal(t, 0, out.ExitCode)
	childPID, err := strconv.Atoi(out.Stdout)
	require.NoError(t, err)
	requireProcessTerminated(t, childPID)
}
