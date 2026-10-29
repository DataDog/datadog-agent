// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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

// runNestedElevation runs script in dir under remediation mode with the given
// allowed and elevatable commands. The elevate callback records each command
// it elevates.
func runNestedElevation(t *testing.T, dir, script string, allowed, elevatable []string) (stderr string, elevatedCalls []string, status int) {
	t.Helper()
	var stderrBuf bytes.Buffer
	prefixed := func(names []string) []string {
		out := make([]string, len(names))
		for i, name := range names {
			out[i] = "rshell:" + name
		}
		return out
	}
	runner, err := New(
		StdIO(nil, nil, &stderrBuf),
		AllowedCommands(prefixed(allowed)),
		AllowedPaths([]string{dir + ":rw"}),
		WithMode(ModeRemediation),
		SelectiveElevation(prefixed(elevatable), func(_ context.Context, command string, run func()) error {
			elevatedCalls = append(elevatedCalls, command)
			run()
			return nil
		}),
	)
	require.NoError(t, err)
	defer runner.Close()
	runner.Dir = dir
	program, err := ParseScript(script, "")
	require.NoError(t, err)
	err = runner.Run(context.Background(), program)
	if err != nil {
		var exit ExitStatus
		require.ErrorAs(t, err, &exit)
		status = int(exit)
	}
	return stderrBuf.String(), elevatedCalls, status
}

// Children that an elevated find -exec / -execdir or xargs dispatches run
// inside the parent's elevation window, so they must hold their own
// ElevatableCommands grant, exactly like a direct `sudo child`.
func TestSelectiveElevationNestedChildRequiresGrant(t *testing.T) {
	for _, tc := range []struct {
		name       string
		script     string
		elevatable []string
	}{
		{"find -exec", `sudo find . -name victim -exec truncate -s 0 {} \;`, []string{"find"}},
		{"find -execdir", `sudo find . -name victim -execdir truncate -s 0 {} \;`, []string{"find"}},
		{"xargs", `sudo xargs -a args truncate -s 0`, []string{"xargs"}},
		{"xargs with stdin", `sudo xargs truncate -s 0 < args`, []string{"xargs"}},
		// xargs holds its own grant, so the gate admits it and the refusal
		// must come from the grandchild dispatch of truncate.
		{"find -exec xargs", `sudo find . -name args -exec xargs -a {} truncate -s 0 \;`, []string{"find", "xargs"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			victim := filepath.Join(dir, "victim")
			require.NoError(t, os.WriteFile(victim, []byte("protected\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "args"), []byte("victim\n"), 0o644))

			stderr, calls, _ := runNestedElevation(t, dir, tc.script,
				[]string{"find", "xargs", "truncate"}, tc.elevatable)

			require.Equal(t, tc.elevatable[:1], calls)
			require.Contains(t, stderr, "rshell: sudo: truncate: elevation not allowed")
			require.NotContains(t, stderr, "xargs: elevation not allowed")
			data, err := os.ReadFile(victim)
			require.NoError(t, err)
			require.Equal(t, "protected\n", string(data), "nested child must not run inside the elevation window")
		})
	}
}

// The direct form is refused the same way, so the nested refusal matches the
// policy the operator configured rather than introducing a new one.
func TestSelectiveElevationDirectChildRefused(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("protected\n"), 0o644))
	stderr, calls, status := runNestedElevation(t, dir, "sudo truncate -s 0 victim",
		[]string{"xargs", "truncate"}, []string{"xargs"})
	require.Empty(t, calls)
	require.Equal(t, 126, status)
	require.Contains(t, stderr, "rshell: sudo: truncate: elevation not allowed")
}

// A child that is itself elevatable keeps working under an elevated parent,
// and children of an unelevated parent are unaffected by the grant.
func TestSelectiveElevationNestedChildAllowed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		script     string
		elevatable []string
		calls      []string
	}{
		{"elevated xargs, elevatable child", "sudo xargs -a args truncate -s 0", []string{"xargs", "truncate"}, []string{"xargs"}},
		{"elevated find, elevatable child", `sudo find . -name victim -exec truncate -s 0 {} \;`, []string{"find", "truncate"}, []string{"find"}},
		{"unelevated xargs", "xargs -a args truncate -s 0", []string{"xargs"}, nil},
		{"unelevated find", `find . -name victim -exec truncate -s 0 {} \;`, []string{"find"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			victim := filepath.Join(dir, "victim")
			require.NoError(t, os.WriteFile(victim, []byte("data\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "args"), []byte("victim\n"), 0o644))

			stderr, calls, status := runNestedElevation(t, dir, tc.script,
				[]string{"find", "xargs", "truncate"}, tc.elevatable)

			require.Equal(t, 0, status, stderr)
			require.Empty(t, stderr)
			require.Equal(t, tc.calls, calls)
			data, err := os.ReadFile(victim)
			require.NoError(t, err)
			require.Empty(t, data)
		})
	}
}
