// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (linux || darwin || windows)

package com_datadoghq_remoteaction_rshell

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// These tests use log capture helpers that exist only with the test build tag.

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logBuffer bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logBuffer, log.DebugLvl)
	require.NoError(t, err)
	previousLogger := log.Default()
	t.Cleanup(func() { log.SetupLogger(previousLogger, "debug") })
	log.SetupLogger(logger, "debug")
	return &logBuffer
}

func TestRunCommandLogsOneSandboxWarningPerRun(t *testing.T) {
	logBuffer := captureLogs(t)

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
	assert.Equal(t, 1, strings.Count(logs, "[WARN] rshell sandbox:"), logs)
	assert.Equal(t, 1, strings.Count(logs, "[WARN] rshell sandbox: 2 allowlist warnings: "), logs)
	assert.NotContains(t, logs, "more, full list at debug level")
	assert.NotContains(t, logs, "rshell sandbox warnings:")
	assert.NotContains(t, logs, "not found, rshell may fail")
	for _, warning := range result.SandboxWarnings {
		assert.Contains(t, logs, warning)
	}
}

func TestRunCommandTruncatesSandboxWarningSummary(t *testing.T) {
	logBuffer := captureLogs(t)

	extra := 3
	paths := make([]string, 0, maxSandboxWarningsInSummary+extra)
	for i := 0; i < maxSandboxWarningsInSummary+extra; i++ {
		paths = append(paths, fmt.Sprintf("/__rshell_sandbox_warnings_test_missing_%d__", i))
	}
	handler := newDefaultRunCommandHandler()

	task := makeTaskWithPaths("echo hello", []string{"rshell:echo"}, paths)

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	require.Len(t, result.SandboxWarnings, len(paths))
	logs := logBuffer.String()

	var warnLine, debugLine string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "[WARN] rshell sandbox:") {
			require.Empty(t, warnLine, "more than one sandbox warning line: %s", logs)
			warnLine = line
		}
		if strings.Contains(line, "[DEBUG] rshell sandbox warnings:") {
			require.Empty(t, debugLine, "more than one debug summary line: %s", logs)
			debugLine = line
		}
	}
	require.NotEmpty(t, warnLine, logs)
	require.NotEmpty(t, debugLine, logs)
	assert.Contains(t, warnLine, fmt.Sprintf("rshell sandbox: %d allowlist warnings: ", len(paths)))
	assert.Contains(t, warnLine, fmt.Sprintf("(and %d more, full list at debug level)", extra))
	for i, warning := range result.SandboxWarnings {
		if i < maxSandboxWarningsInSummary {
			assert.Contains(t, warnLine, warning)
		} else {
			assert.NotContains(t, warnLine, warning)
		}
		assert.Contains(t, debugLine, warning)
	}
}
