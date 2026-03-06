// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_shell

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func makeTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: inputs,
	}
	return task
}

func TestRunShellCommand(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "echo hello world",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello world", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.Greater(t, result.DurationMillis, -1)
}

func TestRunShellScript(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "x=42\necho \"value is $x\"",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "value is 42", result.Stdout)
}

func TestRunShellExitCode(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "exit 42",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 42, result.ExitCode)
}

func TestRunShellStderr(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "echo error message >&2",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Empty(t, result.Stdout)
	assert.Equal(t, "error message", result.Stderr)
}

func TestRunShellPipeline(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "echo 'a b c' | echo ok",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "ok", result.Stdout)
}

func TestRunShellNoInput(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "script must be provided")
}

func TestRunShellSyntaxError(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "if then",
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse shell input")
}

func TestRunShellTimeout(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script":  "while true; do :; done",
		"timeout": 1,
	})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestRunShellBlockedExternalCommand(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "curl http://example.com",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "not allowed")
}

func TestRunShellBuiltinAlwaysAllowed(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "echo builtin-works && pwd",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "builtin-works")
}

func TestRunShellMultiLineScript(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "sum=0\nfor i in 1 2 3; do\n  sum=$((sum + i))\ndone\necho $sum",
	})

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result := output.(*RunShellOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "6", result.Stdout)
}

func TestRunShellCancelledContext(t *testing.T) {
	handler := NewRunShellHandler()
	task := makeTask(map[string]interface{}{
		"script": "while true; do :; done",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := handler.Run(ctx, task, nil)
	require.Error(t, err)
}
