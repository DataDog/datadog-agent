// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewShellScriptCommandRunsNonExecutableScriptFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "hello.sh")
	scriptContents := []byte("#!/bin/sh\nset -e\necho \"Hello, $1\"\n")
	require.NoError(t, os.WriteFile(scriptPath, scriptContents, 0o644))

	cmd, err := NewShellScriptCommand(context.Background(), scriptPath, []string{"World"})
	require.NoError(t, err)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout

	require.NoError(t, cmd.Run())
	require.Equal(t, "Hello, World\n", stdout.String())
}

func TestNewShellScriptCommandBuildsArgsForShellFileExecution(t *testing.T) {
	t.Parallel()

	cmd, err := NewShellScriptCommand(context.Background(), "/tmp/test.sh", []string{"arg1", "arg2"})
	require.NoError(t, err)
	require.Equal(t, []string{"/bin/sh", "/tmp/test.sh", "arg1", "arg2"}, cmd.Args)
}
