// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package com_datadoghq_remoteaction_rshell

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/rshell/interp"
	"github.com/DataDog/rshell/privilegedhelper"
	"google.golang.org/protobuf/proto"
)

func makeTask(command string, allowedCommands []string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]any{"command": command},
		SystemInputs: &privateactionspb.SystemInputs{
			Input: &privateactionspb.SystemInputs_RemoteAction{
				RemoteAction: &privateactionspb.RemoteAction{
					AllowedCommands: allowedCommands,
				},
			},
		},
	}
	return task
}

// makeTaskWithPaths constructs a task carrying the backend allowlists in the
// signed task's nested system_inputs.remote_action policy. Use makeTask
// (without this helper) to exercise the "backend did not send allowed_paths"
// branch: a nil slice.
func makeTaskWithPaths(command string, allowedCommands []string, allowedPaths []string) *types.Task {
	task := makeTask(command, allowedCommands)
	task.Data.Attributes.SystemInputs.GetRemoteAction().AllowedPaths = allowedPaths
	return task
}

func makeLegacyTask(command string, allowedCommands []string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]any{
			"command":         command,
			"allowedCommands": allowedCommands,
		},
	}
	return task
}

func makeLegacyTaskWithPaths(command string, allowedCommands []string, allowedPaths map[string][]string) *types.Task {
	task := makeLegacyTask(command, allowedCommands)
	task.Data.Attributes.Inputs["allowedPaths"] = allowedPaths
	return task
}

func defaultRunCommandHandlerConfig() RunCommandHandlerConfig {
	return RunCommandHandlerConfig{
		OperatorAllowedPaths:    []string{setup.RShellPathAllowAll},
		OperatorAllowedCommands: []string{setup.RShellCommandAllowAllWildcard},
	}
}

func newDefaultRunCommandHandler() *RunCommandHandler {
	return NewRunCommandHandler(defaultRunCommandHandlerConfig())
}

func newDefaultRunRemediationCommandHandler() *RunCommandHandler {
	return NewRunRemediationCommandHandler(defaultRunCommandHandlerConfig())
}

func TestFilterAllowedCommandsUsesBackendPayload(t *testing.T) {
	cases := []struct {
		name    string
		backend []string
		want    []string
	}{
		{
			name:    "nil",
			backend: nil,
			want:    []string{},
		},
		{
			name:    "empty",
			backend: []string{},
			want:    []string{},
		},
		{
			name:    "backend rshell commands are preserved in order",
			backend: []string{"rshell:echo", "rshell:cat"},
			want:    []string{"rshell:echo", "rshell:cat"},
		},
		{
			name:    "non-rshell backend entries are ignored",
			backend: []string{"rshell:echo", "evil:cat", "cat"},
			want:    []string{"rshell:echo"},
		},
		{
			name:    "wildcard token itself is ignored",
			backend: []string{setup.RShellCommandAllowAllWildcard, "rshell:echo"},
			want:    []string{"rshell:echo"},
		},
		{
			name:    "empty rshell command name is ignored",
			backend: []string{"rshell:", "rshell:echo"},
			want:    []string{"rshell:echo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newDefaultRunCommandHandler()

			got := handler.filterAllowedCommands(tc.backend)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestFilterAllowedCommandsDefaultOperatorPolicyEqualsBackendPolicy(t *testing.T) {
	backend := []string{"rshell:echo", "rshell:cat", "evil:curl", "rshell:", setup.RShellCommandAllowAllWildcard}
	handler := newDefaultRunCommandHandler()

	got := handler.filterAllowedCommands(backend)

	assert.Equal(t, onlyRshellPrefixedCommands(backend), got)
}

func TestFilterAllowedCommandsIntersectsAgentAllowlist(t *testing.T) {
	cases := []struct {
		name     string
		agent    []string
		backend  []string
		expected []string
	}{
		{
			name:     "exact command intersection preserves backend order",
			agent:    []string{"rshell:cat", "rshell:grep"},
			backend:  []string{"rshell:echo", "rshell:grep", "rshell:cat"},
			expected: []string{"rshell:grep", "rshell:cat"},
		},
		{
			name:     "disjoint lists produce empty result",
			agent:    []string{"rshell:cat"},
			backend:  []string{"rshell:echo"},
			expected: []string{},
		},
		{
			name:     "explicit empty agent list blocks all backend commands",
			agent:    []string{},
			backend:  []string{"rshell:echo"},
			expected: []string{},
		},
		{
			name:     "agent wildcard leaves backend rshell commands intact",
			agent:    []string{setup.RShellCommandAllowAllWildcard},
			backend:  []string{"rshell:echo", "evil:cat", "rshell:cat"},
			expected: []string{"rshell:echo", "rshell:cat"},
		},
		{
			name:     "unnamespaced agent commands do not match backend rshell commands",
			agent:    []string{"cat"},
			backend:  []string{"rshell:cat"},
			expected: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(RunCommandHandlerConfig{
				OperatorAllowedPaths:    []string{setup.RShellPathAllowAll},
				OperatorAllowedCommands: tc.agent,
			})

			got := handler.filterAllowedCommands(tc.backend)

			if len(tc.expected) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.expected, got)
			}
		})
	}
}

func TestFilterAllowedPathsUsesBackendPayload(t *testing.T) {
	cases := []struct {
		name    string
		backend []string
		want    []string
	}{
		{
			name:    "nil",
			backend: nil,
			want:    []string{},
		},
		{
			name:    "empty",
			backend: []string{},
			want:    []string{},
		},
		{
			name:    "backend paths are normalized",
			backend: []string{"/var/log", "/etc/"},
			want:    []string{"/var/log/", "/etc/"},
		},
		{
			name: "backend paths from multiple policies are reduced",
			backend: []string{
				"/etc/datadog-agent",
				"/etc",
				"/host/var/log",
				"/host/var/log",
				"/host/var/log",
			},
			want: []string{"/etc/", "/host/var/log/"},
		},
		{
			name:    "backend access overlays are preserved",
			backend: []string{"/var/log:ro", "/var/log/datadog:rw"},
			want:    []string{"/var/log/:ro", "/var/log/datadog/:rw"},
		},
		{
			name:    "backend read-write path replaces read-only duplicate with same path",
			backend: []string{"/var/log:ro", "/var/log:rw", "/etc:ro"},
			want:    []string{"/etc/:ro", "/var/log/:rw"},
		},
		{
			name:    "backend dot segments are cleaned without reducing siblings",
			backend: []string{"/var/./log/../log:ro", "/var/logger:rw"},
			want:    []string{"/var/log/:ro", "/var/logger/:rw"},
		},
		{
			name:    "operator root sentinel admits windows drive-rooted backend paths",
			backend: []string{"C:/Users/ContainerAdministrator/AppData/Local/Temp/par:rw"},
			want:    []string{"C:/Users/ContainerAdministrator/AppData/Local/Temp/par/:rw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(RunCommandHandlerConfig{
				OperatorAllowedPaths: []string{setup.RShellPathAllowAll},
			})

			got := handler.filterAllowedPaths(tc.backend)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.ElementsMatch(t, tc.want, got)
			}
		})
	}
}

func TestFilterAllowedPathsDefaultOperatorPolicyEqualsReducedBackendPolicy(t *testing.T) {
	backend := []string{
		"/etc/datadog-agent",
		"/etc",
		"/host/var/log:ro",
		"/host/var/log/datadog:ro",
		"/tmp/cache:rw",
		"/tmp:rw",
		"C:/Users/ContainerAdministrator/AppData/Local/Temp/par:rw",
	}
	handler := newDefaultRunCommandHandler()

	got := handler.filterAllowedPaths(backend)

	assert.Equal(t, reducePathListToBroadest(cleanPathList(backend)), got)
}

func TestFilterAllowedPathsIntersectsAgentAllowlistByAccess(t *testing.T) {
	cases := []struct {
		name     string
		agent    []string
		backend  []string
		expected []string
	}{
		{
			name:     "agent read-only descendant of backend read-only is kept",
			agent:    []string{"/var/log/datadog:ro"},
			backend:  []string{"/var/log:ro"},
			expected: []string{"/var/log/datadog/:ro"},
		},
		{
			name:     "agent read-write descendant of backend read-write is kept",
			agent:    []string{"/var/log/datadog:rw"},
			backend:  []string{"/var/log:rw"},
			expected: []string{"/var/log/datadog/:rw"},
		},
		{
			name:     "equal paths are kept",
			agent:    []string{"/var/log:ro"},
			backend:  []string{"/var/log:ro"},
			expected: []string{"/var/log/:ro"},
		},
		{
			name:     "explicit empty agent list blocks all backend paths",
			agent:    []string{},
			backend:  []string{"/var/log:ro", "/tmp:rw"},
			expected: []string{},
		},
		{
			name:     "backend descendant of agent path is kept",
			agent:    []string{"/var/log:ro"},
			backend:  []string{"/var/log/datadog:ro"},
			expected: []string{"/var/log/datadog/:ro"},
		},
		{
			name:     "explicit agent root keeps narrower backend paths",
			agent:    []string{"/"},
			backend:  []string{"/var/log:ro", "/etc"},
			expected: []string{"/etc/", "/var/log/:ro"},
		},
		{
			name:     "explicit agent root without suffix preserves backend read-write paths",
			agent:    []string{"/"},
			backend:  []string{"/var/log:rw"},
			expected: []string{"/var/log/:rw"},
		},
		{
			name:     "explicit agent root keeps backend read-write over read-only duplicate",
			agent:    []string{"/"},
			backend:  []string{"/var/log:ro", "/var/log:rw"},
			expected: []string{"/var/log/:rw"},
		},
		{
			name:     "agent read-only path can still match backend read-only when same backend read-write also exists",
			agent:    []string{"/var/log:ro"},
			backend:  []string{"/var/log:ro", "/var/log:rw"},
			expected: []string{"/var/log/:ro"},
		},
		{
			name:     "agent read-only and read-write paths keep read-write when same backend path matches both",
			agent:    []string{"/var/log:ro", "/var/log:rw"},
			backend:  []string{"/var/log:ro", "/var/log:rw"},
			expected: []string{"/var/log/:rw"},
		},
		{
			name:     "agent read-write root keeps narrower backend read-write paths",
			agent:    []string{"/:rw"},
			backend:  []string{"/var/log:rw"},
			expected: []string{"/var/log/:rw"},
		},
		{
			name:     "agent read-write path narrows backend read-write path",
			agent:    []string{"/var/log/datadog:rw"},
			backend:  []string{"/var/log:rw"},
			expected: []string{"/var/log/datadog/:rw"},
		},
		{
			name:     "ordinary unsuffixed agent path does not match backend read-write path",
			agent:    []string{"/var/log"},
			backend:  []string{"/var/log/datadog:rw"},
			expected: []string{},
		},
		{
			name:     "unrelated paths are not kept",
			agent:    []string{"/opt/datadog:ro"},
			backend:  []string{"/var/log:ro"},
			expected: []string{},
		},
		{
			name:     "read-only and read-write paths do not cross-match",
			agent:    []string{"/var/log/datadog:rw", "/opt/datadog:ro"},
			backend:  []string{"/var/log:ro", "/opt:rw"},
			expected: []string{},
		},
		{
			name:     "paths without access suffix participate in read-only group",
			agent:    []string{"/var/log/datadog"},
			backend:  []string{"/var/log"},
			expected: []string{"/var/log/datadog/"},
		},
		{
			name:     "read-only and read-write groups are combined after path reduction",
			agent:    []string{"/var/log/datadog:ro", "/opt/datadog:rw", "/tmp/cache:ro"},
			backend:  []string{"/var/log:ro", "/opt:rw", "/tmp:rw"},
			expected: []string{"/opt/datadog/:rw", "/var/log/datadog/:ro"},
		},
		{
			name:     "operator paths are reduced before backend intersection",
			agent:    []string{"/var/log/datadog:rw", "/var/log:rw", "/etc/datadog:ro", "/etc:ro"},
			backend:  []string{"/var/log/datadog/agent:rw", "/etc/datadog/agent:ro"},
			expected: []string{"/etc/datadog/agent/:ro", "/var/log/datadog/agent/:rw"},
		},
		{
			name:     "duplicate backend matches are reduced to broadest path",
			agent:    []string{"/var/log:rw"},
			backend:  []string{"/var/log:rw", "/var/log:rw", "/var/log/datadog:rw"},
			expected: []string{"/var/log/:rw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(RunCommandHandlerConfig{
				OperatorAllowedPaths: tc.agent,
			})

			got := handler.filterAllowedPaths(tc.backend)

			if len(tc.expected) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.expected, got)
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

	NewRunCommandHandler(RunCommandHandlerConfig{
		OperatorAllowedPaths:    paths,
		OperatorAllowedCommands: commands,
	})

	assert.Equal(t, pathsCopy, paths, "AgentAllowedPaths input must not be mutated")
	assert.Equal(t, commandsCopy, commands, "AgentAllowedCommands input must not be mutated")
}

func TestNewRunCommandHandlerReducesOperatorAllowedPathsByAccess(t *testing.T) {
	handler := NewRunCommandHandler(RunCommandHandlerConfig{
		OperatorAllowedPaths: []string{
			"/var/log",
			"/var/log/datadog",
			"/var/log:rw",
			"/var/log/datadog:rw",
			"/etc:ro",
			"/etc/datadog:ro",
		},
	})

	assert.Equal(t, []string{"/etc/:ro", "/var/log/:rw"}, handler.operatorAllowedPaths)
}

func TestRunCommandEmptyCommandReturnsError(t *testing.T) {
	handler := newDefaultRunCommandHandler()

	_, err := handler.Run(context.Background(), makeTask("", nil), nil)

	assert.ErrorContains(t, err, "command is required")
}

func TestRunCommandNoAllowedCommandsBlocksExecution(t *testing.T) {
	// Backend nil → empty effective list → rshell rejects.
	handler := newDefaultRunCommandHandler()

	out, err := handler.Run(context.Background(), makeTask("echo hello", nil), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestPrivilegedExecutionRequiresLocalOptIn(t *testing.T) {
	handler := newDefaultRunRemediationCommandHandler()
	task := makeTask("sudo truncate -s 0 /var/log/app.log", []string{"rshell:truncate"})
	task.Data.Attributes.Inputs["effectivePermissions"] = "EscalationAllowed"
	task.Data.Attributes.Inputs["elevatableCommands"] = []string{"rshell:truncate"}

	_, err := handler.Run(context.Background(), task, nil)
	require.ErrorContains(t, err, "disabled by local configuration")
}

func TestWholeScriptRootIsRejected(t *testing.T) {
	handler := newDefaultRunRemediationCommandHandler()
	task := makeTask("truncate -s 0 /var/log/app.log", []string{"rshell:truncate"})
	task.Data.Attributes.Inputs["effectivePermissions"] = "Root"

	_, err := handler.Run(context.Background(), task, nil)
	require.ErrorContains(t, err, "whole-script root execution is not supported")
}

func TestPrivilegedHelperTaskWireCompatibility(t *testing.T) {
	agentTask := &privateactionspb.PrivateActionTask{
		ActionName:     "runRemediationCommand",
		BundleId:       "com.datadoghq.remoteaction.rshell",
		OrgId:          42,
		TaskId:         "task-1",
		ConnectionInfo: &privateactionspb.ConnectionInfo{RunnerId: "runner-1"},
		SystemInputs: &privateactionspb.SystemInputs{Input: &privateactionspb.SystemInputs_RemoteAction{
			RemoteAction: &privateactionspb.RemoteAction{AllowedCommands: []string{"rshell:truncate"}, AllowedPaths: []string{"/var/log:rw"}},
		}},
	}
	wire, err := proto.Marshal(agentTask)
	require.NoError(t, err)
	var helperTask privilegedhelper.PrivateActionTask
	require.NoError(t, proto.Unmarshal(wire, &helperTask))
	assert.Equal(t, agentTask.ActionName, helperTask.ActionName)
	assert.Equal(t, agentTask.BundleId, helperTask.BundleId)
	assert.Equal(t, agentTask.OrgId, helperTask.OrgId)
	assert.Equal(t, agentTask.TaskId, helperTask.TaskId)
	assert.Equal(t, agentTask.ConnectionInfo.RunnerId, helperTask.ConnectionInfo.RunnerId)
	assert.Equal(t, agentTask.SystemInputs.GetRemoteAction().AllowedCommands, helperTask.SystemInputs.GetRemoteAction().AllowedCommands)
	assert.Equal(t, agentTask.SystemInputs.GetRemoteAction().AllowedPaths, helperTask.SystemInputs.GetRemoteAction().AllowedPaths)
}

func TestRunCommandMissingRemoteActionPolicyBlocksExecution(t *testing.T) {
	handler := newDefaultRunCommandHandler()
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]any{"command": "echo hello"},
	}

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandLegacyInputAllowlistsRemainSupported(t *testing.T) {
	handler := newDefaultRunCommandHandler()

	out, err := handler.Run(context.Background(),
		makeLegacyTask("echo hello", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestRunCommandLegacyInputAllowedPathsRemainSupported(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	payload := dir + "/payload.txt"
	require.NoError(t, os.WriteFile(filepath.FromSlash(payload), []byte("hello\n"), 0o600))
	handler := newDefaultRunCommandHandler()

	out, err := handler.Run(context.Background(),
		makeLegacyTaskWithPaths("cat "+payload,
			[]string{"rshell:cat"},
			map[string][]string{setup.RShellPathAllowMapDefaultKey: {dir}}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestRunCommandSystemInputsOverrideLegacyInputAllowlists(t *testing.T) {
	handler := newDefaultRunCommandHandler()
	task := makeLegacyTask("echo hello", []string{"rshell:echo"})
	task.Data.Attributes.SystemInputs = &privateactionspb.SystemInputs{
		Input: &privateactionspb.SystemInputs_RemoteAction{
			RemoteAction: &privateactionspb.RemoteAction{},
		},
	}

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandWithBackendAllowedCommand(t *testing.T) {
	handler := newDefaultRunCommandHandler()

	out, err := handler.Run(context.Background(),
		makeTask("echo hello", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
}

func TestRunCommandDisallowedCommandBlocked(t *testing.T) {
	// Backend only allowed "rshell:echo"; grep is blocked because it isn't
	// in the signed backend list.
	handler := newDefaultRunCommandHandler()

	out, err := handler.Run(context.Background(),
		makeTask("grep foo", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandAgentCommandAllowlistNarrowsBackendPayload(t *testing.T) {
	handler := NewRunCommandHandler(RunCommandHandlerConfig{
		OperatorAllowedPaths:    []string{setup.RShellPathAllowAll},
		OperatorAllowedCommands: []string{"rshell:cat"},
	})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo", "rshell:cat"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandExplicitEmptyAgentCommandAllowlistBlocksExecution(t *testing.T) {
	handler := NewRunCommandHandler(RunCommandHandlerConfig{
		OperatorAllowedPaths:    []string{setup.RShellPathAllowAll},
		OperatorAllowedCommands: []string{},
	})

	out, err := handler.Run(context.Background(),
		makeTask("echo hi", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandBackendAllowedPathsRestrictsAccess(t *testing.T) {
	// Backend only allows /tmp on bare-metal hosts (the env this test process
	// runs in); /var/log isn't in the backend list, so cat /var/log/syslog
	// must fail.
	handler := newDefaultRunCommandHandler()

	task := makeTaskWithPaths("cat /var/log/syslog",
		[]string{"rshell:cat"},
		[]string{"/tmp"})

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
	// the sandbox configuration noise.
	dir := filepath.ToSlash(t.TempDir())
	missing := "/__rshell_sandbox_warnings_test_missing__"
	handler := newDefaultRunCommandHandler()

	task := makeTaskWithPaths("echo hello",
		[]string{"rshell:echo"},
		[]string{dir, missing})

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
	dir := filepath.ToSlash(t.TempDir())
	handler := newDefaultRunCommandHandler()

	task := makeTaskWithPaths("echo hi",
		[]string{"rshell:echo"},
		[]string{dir})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Nil(t, result.SandboxWarnings,
		"a clean sandbox configuration must produce no warnings")
}

func TestRunCommandPreservesAllowedPathAccessSuffixes(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	handler := newDefaultRunCommandHandler()

	task := makeTaskWithPaths("echo ok",
		[]string{"rshell:echo"},
		[]string{dir + ":rw"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "ok\n", result.Stdout)
	assert.Empty(t, result.Stderr)
	assert.Nil(t, result.SandboxWarnings)
}

func TestRunCommandOutputLimitsReturnActionErrors(t *testing.T) {
	// RunCommandOutputs has no truncation marker. Treat output caps as
	// action errors instead of returning partial stdout/stderr as normal
	// command results.
	dir := filepath.ToSlash(t.TempDir())
	payload := dir + "/payload.txt"
	require.NoError(t, os.WriteFile(
		filepath.FromSlash(payload),
		bytes.Repeat([]byte("x"), 10*1024*1024+1),
		0o600,
	))

	cases := []struct {
		name    string
		command string
		wantErr error
	}{
		{
			name:    "stdout limit",
			command: "cat " + payload,
			wantErr: interp.ErrOutputLimitExceeded,
		},
		{
			name:    "stderr limit",
			command: "cat " + payload + " >&2",
			wantErr: interp.ErrStderrLimitExceeded,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newDefaultRunCommandHandler()
			task := makeTaskWithPaths(tc.command,
				[]string{"rshell:cat"},
				[]string{dir})

			out, err := handler.Run(context.Background(), task, nil)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Nil(t, out)
		})
	}
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

// --- runRemediationCommand ---

// TestNewRshellBundleRegistersBothModes verifies the bundle exposes both
// actions and that each carries the expected rshell execution mode.
func TestNewRshellBundleRegistersBothModes(t *testing.T) {
	bundle := NewRshellBundle(&parconfig.Config{})

	runCommand, ok := bundle.GetAction("runCommand").(*RunCommandHandler)
	require.True(t, ok, "runCommand should be registered")
	assert.Equal(t, interp.ModeReadOnly, runCommand.mode)

	runRemediation, ok := bundle.GetAction("runRemediationCommand").(*RunCommandHandler)
	require.True(t, ok, "runRemediationCommand should be registered")
	assert.Equal(t, interp.ModeRemediation, runRemediation.mode)
}

// TestRunRemediationCommandAllowsFileRedirect verifies that, in remediation
// mode, a file-target output redirection into a path inside the AllowedPaths
// sandbox succeeds and writes the file.
func TestRunRemediationCommandAllowsFileRedirect(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	target := dir + "/out.txt"

	handler := newDefaultRunRemediationCommandHandler()
	task := makeTaskWithPaths("echo hello > "+target,
		[]string{"rshell:echo"},
		[]string{dir + ":rw"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	content, readErr := os.ReadFile(filepath.FromSlash(target))
	require.NoError(t, readErr)
	assert.Equal(t, "hello\n", string(content))
}

// TestRunRemediationCommandReadOnlyPathBlocksFileRedirect verifies that
// remediation mode does not upgrade read-only path entries to read-write.
func TestRunRemediationCommandReadOnlyPathBlocksFileRedirect(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	target := dir + "/out.txt"

	handler := newDefaultRunRemediationCommandHandler()
	task := makeTaskWithPaths("echo hello > "+target,
		[]string{"rshell:echo"},
		[]string{dir + ":ro"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode,
		"remediation mode must reject writes to read-only allowed paths")
	_, statErr := os.Stat(filepath.FromSlash(target))
	assert.True(t, os.IsNotExist(statErr),
		"remediation mode must not create the redirect target for read-only paths")
}

// TestRunCommandReadOnlyBlocksFileRedirect verifies that the read-only
// runCommand handler rejects the same file-target redirection, leaving no
// file behind. This is the security guarantee that distinguishes the two
// actions.
func TestRunCommandReadOnlyBlocksFileRedirect(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	target := dir + "/out.txt"

	handler := newDefaultRunCommandHandler()
	task := makeTaskWithPaths("echo hello > "+target,
		[]string{"rshell:echo"},
		[]string{dir + ":rw"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode,
		"read-only mode must reject file-target redirections")
	_, statErr := os.Stat(filepath.FromSlash(target))
	assert.True(t, os.IsNotExist(statErr),
		"read-only mode must not create the redirect target")
}

// TestRunRemediationCommandRedirectOutsideSandboxBlocked verifies that even
// in remediation mode, a redirection target outside the effective AllowedPaths
// is rejected.
func TestRunRemediationCommandRedirectOutsideSandboxBlocked(t *testing.T) {
	allowedDir := filepath.ToSlash(t.TempDir())
	outsideTarget := filepath.ToSlash(filepath.Join(t.TempDir(), "out.txt"))

	handler := newDefaultRunRemediationCommandHandler()
	// Only allowedDir is in the sandbox; the write targets a sibling temp dir.
	task := makeTaskWithPaths("echo hello > "+outsideTarget,
		[]string{"rshell:echo"},
		[]string{allowedDir + ":rw"})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode,
		"redirection outside the sandbox must be rejected even in remediation mode")
	_, statErr := os.Stat(filepath.FromSlash(outsideTarget))
	assert.True(t, os.IsNotExist(statErr))
}
