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

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
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
	// Operator allowed "rshell:echo"; backend allowed "rshell:echo" and
	// "rshell:cat" — echo should run.
	handler := NewRunCommandHandler(nil, []string{"rshell:echo"})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo", "rshell:cat"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hi\n", result.Stdout)
}

func TestRunCommandOperatorIntersectionBlocksDisjoint(t *testing.T) {
	// Operator allowed "rshell:cat" only; backend allowed "rshell:echo".
	// Intersection is empty, so echo is rejected.
	handler := NewRunCommandHandler(nil, []string{"rshell:cat"})

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

func TestFilterAllowedCommandsNilOperatorPassesThrough(t *testing.T) {
	handler := NewRunCommandHandler(nil, nil)

	got := handler.filterAllowedCommands([]string{"rshell:echo", "rshell:cat"})

	assert.Equal(t, []string{"rshell:echo", "rshell:cat"}, got)
}

func TestFilterAllowedCommandsIntersection(t *testing.T) {
	handler := NewRunCommandHandler(nil, []string{"rshell:echo", "rshell:ls"})

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
	handler := NewRunCommandHandler(nil, []string{"rshell:echo", "rshell:cat"})

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

func TestFilterAllowedPathsIntersection(t *testing.T) {
	handler := NewRunCommandHandler([]string{"/var/log", "/tmp"}, nil)

	got := handler.filterAllowedPaths([]string{"/var/log", "/etc", "/tmp"})

	assert.Equal(t, []string{"/var/log", "/tmp"}, got)
}

func TestFilterAllowedPathsDisjointDropped(t *testing.T) {
	handler := NewRunCommandHandler([]string{"/var/log"}, nil)

	got := handler.filterAllowedPaths([]string{"/etc"})

	assert.Empty(t, got)
}

func TestRunCommandBackendAllowedPathsRestrictsAccess(t *testing.T) {
	// End-to-end: operator allows /var/log, backend lists only /tmp so
	// reading /var/log/syslog must fail because /var/log is absent from
	// the backend side of the intersection.
	handler := NewRunCommandHandler([]string{"/var/log"}, []string{"rshell:cat"})

	task := makeTaskWithPaths("cat /var/log/syslog",
		[]string{"rshell:cat"}, []string{"/tmp"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode, "expected cat to fail because /var/log is not in the backend list")
}

// TestFilterAllowedCommandsMatrix pins every cell of the 3x3 grid
// (backend in {nil, [], non-empty} x operator in {nil, [], non-empty}) with
// four sub-cases splitting the non-empty x non-empty cell by the set
// relationship between operator and backend. Twelve scenarios total. The
// truth table is documented in the PR description; this is its executable
// form.
func TestFilterAllowedCommandsMatrix(t *testing.T) {
	cases := []struct {
		name     string
		backend  []string
		operator []string
		want     []string // nil or empty both mean "nothing allowed"
	}{
		// Backend nil — fail-closed regardless of what the operator said.
		{"backend nil, operator nil", nil, nil, nil},
		{"backend nil, operator empty list", nil, []string{}, nil},
		{"backend nil, operator set", nil, []string{"rshell:echo"}, nil},

		// Backend explicit empty list — same outcome as nil.
		{"backend empty list, operator nil", []string{}, nil, nil},
		{"backend empty list, operator empty list", []string{}, []string{}, nil},
		{"backend empty list, operator set", []string{}, []string{"rshell:echo"}, nil},

		// Backend non-empty: the non-empty x non-empty cell splits into
		// four sub-cases by set relationship.
		{"backend set, operator nil (pass-through)",
			[]string{"rshell:echo", "rshell:cat"}, nil,
			[]string{"rshell:echo", "rshell:cat"}},
		{"backend set, operator empty list (operator blocks all)",
			[]string{"rshell:echo"}, []string{}, nil},
		{"backend set, operator is superset of backend",
			[]string{"rshell:echo", "rshell:cat"}, []string{"rshell:echo", "rshell:cat", "rshell:ls"},
			[]string{"rshell:echo", "rshell:cat"}},
		{"backend set, backend is superset of operator",
			[]string{"rshell:echo", "rshell:cat", "rshell:ls"}, []string{"rshell:echo"},
			[]string{"rshell:echo"}},
		{"backend set, operator partial overlap",
			[]string{"rshell:echo", "rshell:cat"}, []string{"rshell:cat", "rshell:ls"},
			[]string{"rshell:cat"}},
		{"backend set, operator disjoint",
			[]string{"rshell:echo"}, []string{"rshell:cat"}, nil},
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

// TestFilterAllowedPathsMatrix is the paths analogue of
// TestFilterAllowedCommandsMatrix. Path intersection is containment-aware
// ("narrower wins") rather than plain set equality, so the non-empty x
// non-empty cell has extra sub-cases for sub-path interplay.
func TestFilterAllowedPathsMatrix(t *testing.T) {
	cases := []struct {
		name     string
		backend  []string
		operator []string
		want     []string
	}{
		// Backend nil — fail-closed.
		{"backend nil, operator nil", nil, nil, nil},
		{"backend nil, operator empty list", nil, []string{}, nil},
		{"backend nil, operator set", nil, []string{"/var/log"}, nil},

		// Backend explicit empty list — same outcome as nil.
		{"backend empty list, operator nil", []string{}, nil, nil},
		{"backend empty list, operator empty list", []string{}, []string{}, nil},
		{"backend empty list, operator set", []string{}, []string{"/var/log"}, nil},

		// Backend non-empty: four sub-cases for the set relationship.
		{"backend set, operator nil (pass-through)",
			[]string{"/var/log", "/etc"}, nil,
			[]string{"/var/log", "/etc"}},
		{"backend set, operator empty list (operator blocks all)",
			[]string{"/var/log"}, []string{}, nil},
		{"backend set, operator is superset of backend",
			[]string{"/var/log", "/tmp"}, []string{"/var/log", "/tmp", "/etc"},
			[]string{"/var/log", "/tmp"}},
		{"backend set, backend is superset of operator",
			[]string{"/var/log", "/tmp", "/etc"}, []string{"/var/log"},
			[]string{"/var/log"}},
		{"backend set, operator partial overlap",
			[]string{"/var/log", "/opt"}, []string{"/var/log", "/etc"},
			[]string{"/var/log"}},
		{"backend set, operator disjoint",
			[]string{"/etc"}, []string{"/var/log"}, nil},

		// Containment / "narrower wins" cases.
		{"operator narrower than backend (sub-path wins)",
			[]string{"/var/log"}, []string{"/var/log/nginx"},
			[]string{"/var/log/nginx"}},
		{"backend narrower than operator (backend wins)",
			[]string{"/var/log/nginx"}, []string{"/var/log"},
			[]string{"/var/log/nginx"}},
		{"operator selects two siblings under one backend parent",
			[]string{"/var/log"}, []string{"/var/log/nginx", "/var/log/apache"},
			[]string{"/var/log/nginx", "/var/log/apache"}},
		{"trailing slash on operator entry is normalized",
			[]string{"/var/log"}, []string{"/var/log/"},
			[]string{"/var/log/"}},
		{"prefix sibling is not a sub-path (/var/logger vs /var/log)",
			[]string{"/var/log"}, []string{"/var/logger"}, nil},
		{"backend prefix sibling is not a sub-path",
			[]string{"/var/logger"}, []string{"/var/log"}, nil},

		// Windows-native paths: PAR runs on Windows too and rshell itself
		// uses OS-native separators. Containment must work regardless of
		// whether entries use forward or backward slashes.
		{"windows operator narrower than windows backend",
			[]string{`C:\ProgramData\Datadog`}, []string{`C:\ProgramData\Datadog\logs`},
			[]string{`C:\ProgramData\Datadog\logs`}},
		{"windows backend narrower than windows operator",
			[]string{`C:\ProgramData\Datadog\logs`}, []string{`C:\ProgramData\Datadog`},
			[]string{`C:\ProgramData\Datadog\logs`}},
		{"windows prefix sibling is not contained",
			[]string{`C:\Data`}, []string{`C:\DataExtra`}, nil},
		{"mixed separators still intersect",
			[]string{`C:\ProgramData\Datadog`}, []string{"C:/ProgramData/Datadog/logs"},
			[]string{"C:/ProgramData/Datadog/logs"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(tc.operator, nil)

			got := handler.filterAllowedPaths(tc.backend)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestFilterAllowedCommandsRequiresNamespacedForm(t *testing.T) {
	// The intersection is plain string equality: operator entries must be
	// spelled in the backend's namespaced "rshell:<name>" form. A bare
	// name like "cat" is silently ignored.
	handler := NewRunCommandHandler(nil, []string{"cat", "rshell:echo"})

	got := handler.filterAllowedCommands([]string{"rshell:cat", "rshell:echo", "rshell:ls"})

	assert.Equal(t, []string{"rshell:echo"}, got)
}

func TestNewRunCommandHandlerStoresAllowedPaths(t *testing.T) {
	paths := []string{"/var/log", "/tmp"}

	handler := NewRunCommandHandler(paths, nil)

	assert.Equal(t, []string{"/var/log", "/tmp"}, handler.operatorAllowedPaths)
	assert.True(t, handler.operatorPathsFilterEnabled)
}

func TestNewRshellBundleUsesConfiguredAllowedPaths(t *testing.T) {
	cfg := &config.Config{RShellAllowedPaths: []string{"/var/log", "/tmp"}}

	bundle := NewRshellBundle(cfg)
	action := bundle.GetAction("runCommand")

	handler, ok := action.(*RunCommandHandler)
	require.True(t, ok)
	assert.Equal(t, []string{"/var/log", "/tmp"}, handler.operatorAllowedPaths)
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
