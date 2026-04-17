// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func makeTask(command string, allowedCommands []string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"command":         command,
			"allowedCommands": allowedCommands,
		},
	}
	return task
}

// makeTaskWithPaths constructs a task whose inputs include the allowedPaths
// field. Use makeTask (without this helper) to exercise the "backend did not
// send the field" branch — absent JSON fields and explicit null both
// round-trip to a nil Go slice.
func makeTaskWithPaths(command string, allowedCommands, allowedPaths []string) *types.Task {
	task := makeTask(command, allowedCommands)
	task.Data.Attributes.Inputs["allowedPaths"] = allowedPaths
	return task
}

func TestRunCommandEmptyCommandReturnsError(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	_, err := handler.Run(context.Background(), makeTask("", nil), nil)

	assert.ErrorContains(t, err, "command is required")
}

func TestRunCommandNoAllowedCommandsBlocksExecution(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	out, err := handler.Run(context.Background(), makeTask("echo hello", nil), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandWithAllowedCommandSucceeds(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	out, err := handler.Run(context.Background(), makeTask("echo hello", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestRunCommandDisallowedCommandBlocked(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	out, err := handler.Run(context.Background(), makeTask("grep foo", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandOperatorIntersectionAllows(t *testing.T) {
	// Operator allowed "echo"; backend allowed "echo" and "cat" — echo should run.
	handler := NewRunCommandHandler(nil, []string{"echo"})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo", "rshell:cat"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hi\n", result.Stdout)
}

func TestRunCommandOperatorIntersectionBlocksDisjoint(t *testing.T) {
	// Operator allowed "cat" only; backend allowed "echo". Intersection is
	// empty, so echo is rejected even though the backend approved it.
	handler := NewRunCommandHandler(nil, []string{"cat"})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandOperatorEmptyListBlocksEverything(t *testing.T) {
	// Operator explicitly set an empty allowlist — nothing runs, even when
	// the backend approved commands.
	handler := NewRunCommandHandler(nil, []string{})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandOperatorAcceptsPrefixedNames(t *testing.T) {
	// Operator wrote "rshell:echo" instead of bare "echo" — still works.
	handler := NewRunCommandHandler(nil, []string{"rshell:echo"})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hi\n", result.Stdout)
}

func TestFilterAllowedCommandsNilOperatorPassesThrough(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	got := handler.filterAllowedCommands([]string{"rshell:echo", "rshell:cat"})

	assert.Equal(t, []string{"rshell:echo", "rshell:cat"}, got)
}

func TestFilterAllowedCommandsIntersection(t *testing.T) {
	handler := NewRunCommandHandler(nil, []string{"echo", "ls"})

	got := handler.filterAllowedCommands([]string{"rshell:echo", "rshell:cat", "rshell:ls"})

	assert.Equal(t, []string{"rshell:echo", "rshell:ls"}, got)
}

func TestFilterAllowedPathsNilBackendBlocksAll(t *testing.T) {
	// Backend did not send the field — fail closed. The operator cannot
	// grant filesystem access the backend withheld.
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths(nil)

	assert.Empty(t, got)
}

func TestFilterAllowedPathsOperatorUnsetPassesThrough(t *testing.T) {
	// Operator left allowed_paths unset in datadog.yaml — the backend list
	// passes through unchanged (no operator-side tightening).
	handler := NewRunCommandHandler(nil, nil)

	got := handler.filterAllowedPaths([]string{"/var/log/nginx", "/etc"})

	assert.Equal(t, []string{"/var/log/nginx", "/etc"}, got)
}

func TestFilterAllowedCommandsNilBackendBlocksAll(t *testing.T) {
	// Same principle for commands: no backend list → rshell blocks all,
	// regardless of what the operator configured.
	handler := NewRunCommandHandler(nil, []string{"echo", "cat"})

	got := handler.filterAllowedCommands(nil)

	assert.Empty(t, got)
}

func TestFilterAllowedPathsExplicitEmptyBackendBlocksAll(t *testing.T) {
	// Backend explicitly sent []. Distinct from the nil case: it signals
	// "the backend chose to restrict everything". rshell will block access.
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{})

	assert.Empty(t, got)
}

func TestFilterAllowedPathsBackendNarrowerWins(t *testing.T) {
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{"/var/log/nginx"})

	assert.Equal(t, []string{"/var/log/nginx"}, got)
}

func TestFilterAllowedPathsOperatorNarrowerWins(t *testing.T) {
	handler := NewRunCommandHandler([]string{"/var/log/nginx"}, nil)

	got := handler.filterAllowedPaths([]string{"/var/log"})

	assert.Equal(t, []string{"/var/log/nginx"}, got)
}

func TestFilterAllowedPathsDisjointDropped(t *testing.T) {
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{"/etc"})

	assert.Empty(t, got)
}

func TestFilterAllowedPathsPrefixSiblingNotOverlap(t *testing.T) {
	// "/var/logger" must not be considered under "/var/log".
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{"/var/logger"})

	assert.Empty(t, got)
}

func TestFilterAllowedPathsMultiPath(t *testing.T) {
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{"/var/log/nginx", "/var/log/postgres", "/etc/passwd"})

	assert.Equal(t, []string{"/var/log/nginx", "/var/log/postgres"}, got)
}

func TestFilterAllowedPathsNormalizesSlashes(t *testing.T) {
	// Trailing slashes and duplicated separators on either side should not
	// affect the ancestor check.
	handler := NewRunCommandHandler([]string{"/var/log/"}, nil)

	got := handler.filterAllowedPaths([]string{"/var//log/nginx"})

	assert.Equal(t, []string{"/var/log/nginx"}, got)
}

func TestFilterAllowedPathsDedupes(t *testing.T) {
	// When both operator and backend list the same path, and the backend
	// also lists a subpath of it, we should not emit the shared ancestor
	// twice.
	handler := NewRunCommandHandler([]string{"/var/log", "/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{"/var/log/nginx"})

	assert.Equal(t, []string{"/var/log/nginx"}, got)
}

func TestRunCommandBackendAllowedPathsRestrictsAccess(t *testing.T) {
	// End-to-end: operator allows /var/log, backend restricts to
	// /var/log/nginx; reading /var/log/postgres must fail.
	handler := NewRunCommandHandler([]string{"/var/log"}, []string{"cat"})

	task := makeTaskWithPaths("cat /var/log/postgres/query.log",
		[]string{"rshell:cat"}, []string{"/var/log/nginx"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode, "expected cat to fail because /var/log/postgres is outside the intersection")
}

func TestNewRunCommandHandlerStoresAllowedPaths(t *testing.T) {
	paths := []string{"/var/log", "/tmp"}

	handler := NewRunCommandHandler(paths, nil)

	assert.Equal(t, paths, handler.operatorAllowedPaths)
}

func TestNewRshellBundleUsesConfiguredAllowedPaths(t *testing.T) {
	paths := []string{"/var/log", "/tmp"}

	bundle := NewRshellBundle(paths, nil)
	action := bundle.GetAction("runCommand")

	handler, ok := action.(*RunCommandHandler)
	require.True(t, ok)
	assert.Equal(t, paths, handler.operatorAllowedPaths)
}

func mockStatFn(existing map[string]bool) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if existing[path] {
			return nil, nil
		}
		return nil, errors.New("not found")
	}
}

func overrideStatFn(t *testing.T, fn func(string) (os.FileInfo, error)) {
	original := statFn
	statFn = fn
	t.Cleanup(func() { statFn = original })
}

func TestResolveProcPathBareMetal(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}

func TestResolveProcPathContainerizedWithHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")
	overrideStatFn(t, mockStatFn(map[string]bool{"/host/proc": true}))

	result := resolveProcPath()

	assert.Equal(t, "/host/proc", result)
}

func TestResolveProcPathContainerizedWithoutHostMount(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")
	overrideStatFn(t, mockStatFn(map[string]bool{}))

	result := resolveProcPath()

	assert.Equal(t, "/proc", result)
}
