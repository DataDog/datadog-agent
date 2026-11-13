// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build private_runner_experimental && !windows

package com_datadoghq_script

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestShellScriptHandlerExecutionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		noFail       bool
		timeout      int
		wantError    string
	}{
		{name: "nonzero exit", script: "exit 7", wantError: "failed to execute"},
		{name: "allowed nonzero exit", script: "exit 7", noFail: true},
		{name: "timeout", script: "sleep 60", noFail: true, timeout: 1, wantError: "timed out after 1 seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &types.Task{}
			task.Data.Attributes = &types.Attributes{Inputs: map[string]interface{}{
				"script": tc.script, "noFailOnError": tc.noFail, "timeout": tc.timeout,
			}}
			output, err := NewRunShellScriptHandler().Run(context.Background(), task, nil)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, output)
			} else {
				require.NoError(t, err)
				require.Equal(t, 7, output.(*RunShellScriptOutputs).ExitCode)
			}
		})
	}
}
