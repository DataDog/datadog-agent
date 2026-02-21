// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestHeadFileHandler(t *testing.T) {
	// Create a temp file that simulates a host-mounted log.
	// We need to place it under /host to match sanitizeAndResolvePath expectations.
	// Since /host won't exist in test, we test the handler logic by calling
	// headFile directly on a temp file.
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "app.log")
	content := "first\nsecond\nthird\nfourth\nfifth\n"
	require.NoError(t, os.WriteFile(logFile, []byte(content), 0644))

	t.Run("headFile reads first N lines", func(t *testing.T) {
		lines, count, err := headFile(logFile, 3)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
		assert.Equal(t, "first\nsecond\nthird", lines)
	})

	t.Run("headFile with default line count", func(t *testing.T) {
		lineCount := clampLineCount(0)
		lines, count, err := headFile(logFile, lineCount)
		require.NoError(t, err)
		assert.Equal(t, 5, count) // file has 5 lines, default is 10
		assert.Contains(t, lines, "first")
		assert.Contains(t, lines, "fifth")
	})
}

func TestHeadFileHandler_ExtractInputs(t *testing.T) {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"filePath":  "/var/log/myapp.log",
			"lineCount": float64(5), // JSON numbers unmarshal as float64
		},
	}

	inputs, err := types.ExtractInputs[headFileInputs](task)
	require.NoError(t, err)
	assert.Equal(t, "/var/log/myapp.log", inputs.FilePath)
	assert.Equal(t, 5, inputs.LineCount)
}

func TestHeadFileHandler_Run_InvalidPath(t *testing.T) {
	handler := NewHeadFileHandler()

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"filePath": "relative/path.log",
		},
	}

	_, err := handler.Run(context.Background(), task, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path must be absolute")
}
