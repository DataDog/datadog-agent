// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectiveElevation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := ""
	runner, err := New(
		StdIO(nil, &stdout, &stderr),
		AllowedCommands([]string{"rshell:echo"}),
		SelectiveElevation([]string{"rshell:echo"}, func(_ context.Context, command string, run func()) error {
			called = command
			run()
			return nil
		}),
	)
	require.NoError(t, err)
	defer runner.Close()
	program, err := ParseScript("sudo echo hello", "")
	require.NoError(t, err)
	require.NoError(t, runner.Run(context.Background(), program))
	require.Equal(t, "echo", called)
	require.Equal(t, "hello\n", stdout.String())
}

func TestSelectiveElevationDefaultDeny(t *testing.T) {
	var stderr bytes.Buffer
	runner, err := New(StdIO(nil, nil, &stderr), AllowedCommands([]string{"rshell:echo"}))
	require.NoError(t, err)
	defer runner.Close()
	program, err := ParseScript("sudo echo hello", "")
	require.NoError(t, err)
	err = runner.Run(context.Background(), program)
	var status ExitStatus
	require.ErrorAs(t, err, &status)
	require.Equal(t, ExitStatus(126), status)
	require.Contains(t, stderr.String(), "elevation not allowed")
}

func TestSelectiveElevationRejectsExpandedMarkerInPipeline(t *testing.T) {
	var stderr bytes.Buffer
	called := false
	runner, err := New(
		StdIO(nil, nil, &stderr),
		AllowedCommands([]string{"rshell:echo"}),
		SelectiveElevation([]string{"rshell:echo"}, func(_ context.Context, _ string, run func()) error {
			called = true
			run()
			return nil
		}),
	)
	require.NoError(t, err)
	defer runner.Close()
	program, err := ParseScript("marker=sudo; $marker echo elevated | echo ordinary", "")
	require.NoError(t, err)
	err = runner.Run(context.Background(), program)
	require.NoError(t, err) // pipeline status is the right-hand command's status
	require.False(t, called)
	require.Contains(t, stderr.String(), "not allowed in pipelines")
}

// A (…) subshell resets the telemetry-only span suppression inside a pipeline
// stage, but the stage still runs concurrently with its siblings, so elevation
// must remain refused there. Otherwise the elevation callback (which lifts the
// effective UID process-wide) would run while an unelevated sibling stage is
// executing.
func TestSelectiveElevationRejectsMarkerInSubshellPipelineStage(t *testing.T) {
	for _, script := range []string{
		"marker=sudo; ($marker echo elevated) | echo ordinary",
		"echo ordinary | (sudo echo elevated)",
		"(sudo echo elevated) | echo ordinary",
		"( (sudo echo elevated) ) | echo ordinary",
		"{ (sudo echo elevated); } | echo ordinary",
		"echo a | echo b | (sudo echo elevated)",
		"x=$( (sudo echo elevated) ) | echo ordinary",
	} {
		t.Run(script, func(t *testing.T) {
			var stderr bytes.Buffer
			called := false
			runner, err := New(
				StdIO(nil, nil, &stderr),
				AllowedCommands([]string{"rshell:echo"}),
				SelectiveElevation([]string{"rshell:echo"}, func(_ context.Context, _ string, run func()) error {
					called = true
					run()
					return nil
				}),
			)
			require.NoError(t, err)
			defer runner.Close()
			program, err := ParseScript(script, "")
			require.NoError(t, err)
			_ = runner.Run(context.Background(), program)
			require.False(t, called)
			require.Contains(t, stderr.String(), "not allowed in pipelines")
		})
	}
}

func TestSelectiveElevationAllowedInStandaloneSubshell(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	runner, err := New(
		StdIO(nil, &stdout, &stderr),
		AllowedCommands([]string{"rshell:echo"}),
		SelectiveElevation([]string{"rshell:echo"}, func(_ context.Context, _ string, run func()) error {
			called = true
			run()
			return nil
		}),
	)
	require.NoError(t, err)
	defer runner.Close()
	program, err := ParseScript("(sudo echo elevated)", "")
	require.NoError(t, err)
	require.NoError(t, runner.Run(context.Background(), program))
	require.True(t, called)
	require.Equal(t, "elevated\n", stdout.String())
}
