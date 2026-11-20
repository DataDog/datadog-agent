// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (linux || darwin || windows)

package com_datadoghq_remoteaction_rshell

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This test uses log capture helpers that exist only with the test build tag.
func TestRunCommandLogsSandboxWarningsOnceAtDebugLevel(t *testing.T) {
	var logBuffer bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logBuffer, log.DebugLvl)
	require.NoError(t, err)
	previousLogger := log.Default()
	t.Cleanup(func() { log.SetupLogger(previousLogger, "debug") })
	log.SetupLogger(logger, "debug")

	dir := filepath.ToSlash(t.TempDir())
	file := dir + "/file.txt"
	require.NoError(t, os.WriteFile(filepath.FromSlash(file), []byte("x"), 0o600))
	missing := "/__rshell_sandbox_warnings_test_missing__"
	handler := newDefaultRunCommandHandler()

	task := makeTaskWithPaths("echo hello",
		[]string{"rshell:echo"},
		[]string{file, missing})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	require.Len(t, result.SandboxWarnings, 2)
	logs := logBuffer.String()
	assert.NotContains(t, logs, "[WARN]")
	assert.Equal(t, 1, strings.Count(logs, "[DEBUG] rshell sandbox warnings:"))
	for _, warning := range result.SandboxWarnings {
		assert.Contains(t, logs, warning)
	}
}
