// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func makeTask(command string, allowedCommands []string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]any{
			"command":         command,
			"allowedCommands": allowedCommands,
		},
	}
	return task
}

// makeTaskWithPaths constructs a task whose inputs include the allowedPaths
// field. The backend ships allowedPaths as a per-environment map keyed by
// "default" / "containerized"; the runner picks the relevant slice based
// on env.IsContainerized at task time. Use makeTask (without this helper)
// to exercise the "backend did not send the field" branch — absent JSON
// fields and explicit null both round-trip to a nil Go map.
func makeTaskWithPaths(command string, allowedCommands []string, allowedPaths map[string][]string) *types.Task {
	task := makeTask(command, allowedCommands)
	task.Data.Attributes.Inputs["allowedPaths"] = allowedPaths
	return task
}

// TestFilterAllowedCommandsMatrix pins backend × operator combinations.
// The match is plain string equality except for the "rshell:*" sentinel,
// which admits every backend entry in the rshell namespace.
func TestFilterAllowedCommandsMatrix(t *testing.T) {
	cases := []struct {
		name     string
		backend  []string
		operator []string
		want     []string
	}{
		// Empty/nil short-circuits.
		{
			name:     "backend nil, operator wildcard",
			backend:  nil,
			operator: []string{setup.RShellCommandAllowAllWildcard},
			want:     []string{},
		},
		{
			name:     "backend nil, operator empty",
			backend:  nil,
			operator: []string{},
			want:     []string{},
		},
		{
			name:     "backend nil, operator set",
			backend:  nil,
			operator: []string{"rshell:echo"},
			want:     []string{},
		},
		{
			name:     "backend empty, operator wildcard",
			backend:  []string{},
			operator: []string{setup.RShellCommandAllowAllWildcard},
			want:     []string{},
		},
		{
			name:     "backend set, operator nil (handler treats as kill-switch)",
			backend:  []string{"rshell:echo"},
			operator: nil,
			want:     []string{},
		},
		{
			name:     "backend set, operator empty (kill-switch)",
			backend:  []string{"rshell:echo"},
			operator: []string{},
			want:     []string{},
		},
		// Wildcard branch.
		{
			name:     "wildcard admits all rshell-prefixed backend entries",
			backend:  []string{"rshell:echo", "rshell:cat"},
			operator: []string{setup.RShellCommandAllowAllWildcard},
			want:     []string{"rshell:echo", "rshell:cat"},
		},
		{
			name:     "wildcard scoped: non-namespaced backend entry rejected",
			backend:  []string{"rshell:echo", "evil:cat"},
			operator: []string{setup.RShellCommandAllowAllWildcard},
			want:     []string{"rshell:echo"}},
		{
			name:     "wildcard coexists with explicit entries (wildcard subsumes them)",
			backend:  []string{"rshell:echo", "rshell:cat"},
			operator: []string{setup.RShellCommandAllowAllWildcard, "rshell:echo"},
			want:     []string{"rshell:echo", "rshell:cat"}},

		// Exact-match intersection.
		{
			name:     "operator superset of backend",
			backend:  []string{"rshell:echo", "rshell:cat"},
			operator: []string{"rshell:echo", "rshell:cat", "rshell:ls"},
			want:     []string{"rshell:echo", "rshell:cat"}},
		{
			name:     "backend superset of operator",
			backend:  []string{"rshell:echo", "rshell:cat", "rshell:ls"},
			operator: []string{"rshell:echo"},
			want:     []string{"rshell:echo"}},
		{
			name:     "partial overlap",
			backend:  []string{"rshell:echo", "rshell:cat"},
			operator: []string{"rshell:cat", "rshell:ls"},
			want:     []string{"rshell:cat"}},
		{
			name:     "disjoint",
			backend:  []string{"rshell:echo"},
			operator: []string{"rshell:cat"},
			want:     []string{}},
		{
			name:     "bare-name operator entry never matches namespaced backend",
			backend:  []string{"rshell:cat"},
			operator: []string{"cat"},
			want:     []string{}},
		{
			name:     "output preserves backend iteration order",
			backend:  []string{"rshell:ls", "rshell:cat", "rshell:echo"},
			operator: []string{"rshell:cat", "rshell:echo", "rshell:ls"},
			want:     []string{"rshell:ls", "rshell:cat", "rshell:echo"}},
		{
			name:     "duplicate operator entries are deduped at handler creation",
			backend:  []string{"rshell:echo"},
			operator: []string{"rshell:echo", "rshell:echo"},
			want:     []string{"rshell:echo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(nil, tc.operator)

			got := handler.filterAllowedCommands(tc.backend)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// TestFilterAllowedPathsMatrix pins backend × operator combinations for
// containment-aware intersection. The function is pure: it takes the
// already-selected per-environment slice (env-routing happens in
// backendPathsForEnv, tested separately). Operator paths are stored in
// cleaned form (path.Clean + trailing "/"), and backend is normalized
// inside filterAllowedPaths, so all expected outputs in this matrix carry
// trailing slashes.
func TestFilterAllowedPathsMatrix(t *testing.T) {
	cases := []struct {
		name     string
		backend  []string
		operator []string
		want     []string
	}{
		// Empty/nil short-circuits.
		{
			name:     "backend nil, operator wildcard",
			backend:  nil,
			operator: []string{setup.RShellPathAllowAll},
			want:     []string{},
		},
		{
			name:     "backend nil, operator empty",
			backend:  nil,
			operator: []string{},
			want:     []string{},
		},
		{
			name:     "backend nil, operator set",
			backend:  nil,
			operator: []string{"/var/log"},
			want:     []string{},
		},
		{
			name:     "backend empty, operator wildcard",
			backend:  []string{},
			operator: []string{setup.RShellPathAllowAll},
			want:     []string{},
		},
		{
			name:     "backend set, operator nil (kill-switch)",
			backend:  []string{"/var/log"},
			operator: nil,
			want:     []string{},
		},
		{
			name:     "backend set, operator empty (kill-switch)",
			backend:  []string{"/var/log"},
			operator: []string{},
			want:     []string{},
		},

		// Wildcard branch — operator "/" passes the backend through.
		// Output is the cleaned/reduced backend list (sorted, trailing "/").
		{
			name:     "wildcard root operator passes backend through",
			backend:  []string{"/var/log", "/etc"},
			operator: []string{"/"},
			want:     []string{"/etc/", "/var/log/"},
		},

		// Exact-match (after normalization).
		{
			name:     "operator superset of backend",
			backend:  []string{"/var/log", "/tmp"},
			operator: []string{"/var/log", "/tmp", "/etc"},
			want:     []string{"/tmp/", "/var/log/"},
		},
		{
			name:     "backend superset of operator",
			backend:  []string{"/var/log", "/tmp", "/etc"},
			operator: []string{"/var/log"},
			want:     []string{"/var/log/"},
		},
		{
			name:     "partial overlap",
			backend:  []string{"/var/log", "/opt"},
			operator: []string{"/var/log", "/etc"},
			want:     []string{"/var/log/"},
		},
		{
			name:     "disjoint",
			backend:  []string{"/etc"},
			operator: []string{"/var/log"},
			want:     []string{},
		},

		// Containment / "narrower wins".
		{
			name:     "operator narrower than backend",
			backend:  []string{"/var/log"},
			operator: []string{"/var/log/nginx"},
			want:     []string{"/var/log/nginx/"},
		},
		{
			name:     "backend narrower than operator",
			backend:  []string{"/var/log/nginx"},
			operator: []string{"/var/log"},
			want:     []string{"/var/log/nginx/"},
		},
		{
			name:     "operator selects two siblings under one backend parent",
			backend:  []string{"/var/log"},
			operator: []string{"/var/log/nginx", "/var/log/apache"},
			want:     []string{"/var/log/apache/", "/var/log/nginx/"},
		},
		{
			name:     "trailing slash on operator entry is normalized",
			backend:  []string{"/var/log"},
			operator: []string{"/var/log/"},
			want:     []string{"/var/log/"},
		},

		// Prefix-sibling rejection.
		{
			name:     "prefix sibling: /var/logger does not satisfy /var/log",
			backend:  []string{"/var/log"},
			operator: []string{"/var/logger"},
			want:     []string{},
		},
		{
			name:     "prefix sibling reversed",
			backend:  []string{"/var/logger"},
			operator: []string{"/var/log"},
			want:     []string{},
		},

		// Operator-side reduction: redundant operator entries collapse.
		{
			name:     "operator entries one inside the other collapse to broader",
			backend:  []string{"/var/log/nginx"},
			operator: []string{"/var/log", "/var/log/nginx"},
			want:     []string{"/var/log/nginx/"},
		},

		// Multi-narrower stress (regression for the intersection bug).
		{
			name:     "operator broad, backend has many narrower siblings — all admitted",
			backend:  []string{"/var/a", "/var/b", "/var/c"},
			operator: []string{"/var"},
			want:     []string{"/var/a/", "/var/b/", "/var/c/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(tc.operator, nil)

			got := handler.filterAllowedPaths(tc.backend)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.ElementsMatch(t, tc.want, got)
			}
		})
	}
}

// TestNewRunCommandHandlerDoesNotMutateInputs guards against the
// constructor sorting (or otherwise reordering) the caller's slices in
// place. The bundle wiring in entrypoint.go passes the same slice we read
// from cfg, so any mutation here would leak into the rest of the agent.
func TestNewRunCommandHandlerDoesNotMutateInputs(t *testing.T) {
	paths := []string{"/var/log", "/etc"}
	commands := []string{"rshell:zls", "rshell:cat", "rshell:cat"}
	pathsCopy := slices.Clone(paths)
	commandsCopy := slices.Clone(commands)

	NewRunCommandHandler(paths, commands)

	assert.Equal(t, pathsCopy, paths, "operatorAllowedPaths input must not be mutated")
	assert.Equal(t, commandsCopy, commands, "operatorAllowedCommands input must not be mutated")
}

func TestNewRunCommandHandlerNormalizesOperatorPaths(t *testing.T) {
	// Cleanup+reduce in action: redundant entries collapse, paths get
	// a trailing slash, and the result is sorted.
	handler := NewRunCommandHandler(
		[]string{"/var/log/nginx", "/var/log", "/etc/"},
		nil,
	)

	assert.Equal(t, []string{"/etc/", "/var/log/"}, handler.operatorAllowedPaths)
}

func TestNewRunCommandHandlerDedupesOperatorCommands(t *testing.T) {
	// Sort+Compact yields a sorted, deduplicated slice. Order is the
	// implementation detail; what matters is "no duplicates."
	handler := NewRunCommandHandler(
		nil,
		[]string{"rshell:cat", "rshell:echo", "rshell:cat", "rshell:ls"},
	)

	assert.Equal(t,
		[]string{"rshell:cat", "rshell:echo", "rshell:ls"},
		handler.operatorAllowedCommands,
	)
}

func TestNewRunCommandHandlerNilInputs(t *testing.T) {
	// Both sides nil: handler treats both as kill-switches downstream.
	handler := NewRunCommandHandler(nil, nil)

	assert.Empty(t, handler.operatorAllowedPaths)
	assert.Empty(t, handler.operatorAllowedCommands)
}

func TestNewRshellBundleUsesConfiguredAllowedPaths(t *testing.T) {
	cfg := &config.Config{RShellAllowedPaths: []string{"/var/log", "/tmp"}}

	bundle := NewRshellBundle(cfg)
	action := bundle.GetAction("runCommand")

	handler, ok := action.(*RunCommandHandler)
	require.True(t, ok)
	// Bundle wires the cleaned/reduced form into the handler.
	assert.Equal(t, []string{"/tmp/", "/var/log/"}, handler.operatorAllowedPaths)
}

func TestRunCommandEmptyCommandReturnsError(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	_, err := handler.Run(context.Background(), makeTask("", nil), nil)

	assert.ErrorContains(t, err, "command is required")
}

func TestRunCommandNoAllowedCommandsBlocksExecution(t *testing.T) {
	// Operator nil + backend nil → empty effective list → rshell rejects.
	handler := NewRunCommandHandler(nil, nil)

	out, err := handler.Run(context.Background(), makeTask("echo hello", nil), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandWithWildcardOperatorAndBackendAllowed(t *testing.T) {
	// Operator uses the default ["rshell:*"] wildcard sentinel; backend
	// allowed "rshell:echo"; echo runs.
	handler := NewRunCommandHandler(nil, []string{setup.RShellCommandAllowAllWildcard})

	out, err := handler.Run(context.Background(),
		makeTask("echo hello", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestRunCommandDisallowedCommandBlocked(t *testing.T) {
	// Operator wildcard, but backend only allowed "rshell:echo"; grep is
	// blocked because it isn't in the backend list.
	handler := NewRunCommandHandler(nil, []string{setup.RShellCommandAllowAllWildcard})

	out, err := handler.Run(context.Background(),
		makeTask("grep foo", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandOperatorIntersectionAllows(t *testing.T) {
	// Operator narrowed to "rshell:echo"; backend allowed echo and cat;
	// echo passes the intersection.
	handler := NewRunCommandHandler(nil, []string{"rshell:echo"})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo", "rshell:cat"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hi\n", result.Stdout)
}

func TestRunCommandOperatorIntersectionBlocksDisjoint(t *testing.T) {
	// Operator narrowed to "rshell:cat"; backend allowed only echo —
	// disjoint, intersection empty, echo rejected.
	handler := NewRunCommandHandler(nil, []string{"rshell:cat"})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandOperatorEmptyListBlocksEverything(t *testing.T) {
	// Explicit empty operator command list is the kill-switch.
	handler := NewRunCommandHandler(nil, []string{})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandBackendAllowedPathsRestrictsAccess(t *testing.T) {
	// End-to-end: operator allows /var/log, backend only allows /tmp on
	// bare-metal hosts (the env this test process runs in); /var/log
	// isn't in the backend list, so cat /var/log/syslog must fail.
	handler := NewRunCommandHandler([]string{"/var/log"}, []string{"rshell:cat"})

	task := makeTaskWithPaths("cat /var/log/syslog",
		[]string{"rshell:cat"},
		map[string][]string{setup.RShellPathAllowMapDefaultKey: {"/tmp"}})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode,
		"expected cat to fail because /var/log is not in the backend list")
}

func TestRunCommandSandboxWarningsKeepStderrClean(t *testing.T) {
	// AllowedPaths includes one valid + one missing entry. The rshell
	// library emits a "skipping" diagnostic for the missing one. The
	// handler must surface that diagnostic in SandboxWarnings, not in
	// Stderr — so callers inspecting Stderr to detect command failure
	// don't see false positives. ExitCode and Stdout are independent of
	// the sandbox configuration noise. The missing path is chosen so it
	// does not share a prefix with the temp dir; otherwise
	// reducePathListToBroadest collapses them and rshell never sees the
	// missing one.
	dir := t.TempDir()
	missing := "/__rshell_sandbox_warnings_test_missing__"
	handler := NewRunCommandHandler([]string{setup.RShellPathAllowAll}, []string{"rshell:echo"})

	task := makeTaskWithPaths("echo hello",
		[]string{"rshell:echo"},
		map[string][]string{setup.RShellPathAllowMapDefaultKey: {dir, missing}})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Empty(t, result.Stderr,
		"sandbox warnings must not leak into the command's stderr field")
	require.Len(t, result.SandboxWarnings, 1,
		"the missing path should produce exactly one warning")
	assert.Contains(t, result.SandboxWarnings[0], "AllowedPaths: skipping")
}

func TestRunCommandSandboxWarningsNilWhenCleanConfig(t *testing.T) {
	// All configured paths exist — SandboxWarnings must be nil so the
	// JSON wire output omits the field entirely (omitempty).
	dir := t.TempDir()
	handler := NewRunCommandHandler([]string{setup.RShellPathAllowAll}, []string{"rshell:echo"})

	task := makeTaskWithPaths("echo hi",
		[]string{"rshell:echo"},
		map[string][]string{setup.RShellPathAllowMapDefaultKey: {dir}})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Nil(t, result.SandboxWarnings,
		"a clean sandbox configuration must produce no warnings")
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
