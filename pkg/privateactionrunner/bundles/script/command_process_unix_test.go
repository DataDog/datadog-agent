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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func processTestCommand(t *testing.T, ctx context.Context, exe string, args []string) *exec.Cmd {
	t.Helper()
	cmd, err := NewPredefinedScriptCommand(ctx, append([]string{exe}, args...), nil)
	require.NoError(t, err)
	return cmd
}

func processTestConfig(exe string, args []string) ScriptBundleConfig {
	return ScriptBundleConfig{SchemaId: schemaIdV1, RunPredefinedScript: map[string]RunPredefinedScriptConfig{
		"test": {Command: append([]string{exe}, args...)},
	}}
}

func timeoutTestConfig() ScriptBundleConfig {
	return processTestConfig("/bin/sh", []string{"-c", "sleep 60"})
}

// processStopped counts a zombie as stopped, since container PID 1 may never reap it.
func processStopped(pid int) bool {
	if runtime.GOOS != "linux" {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	_, state, ok := strings.Cut(string(data), ") ")
	return ok && strings.HasPrefix(state, "Z ")
}
