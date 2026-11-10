// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"context"
	"os/exec"
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
