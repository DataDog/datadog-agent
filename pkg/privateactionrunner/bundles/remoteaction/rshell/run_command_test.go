// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/rshell/interp"
)

func makeTask(command string, allowedCommands []string) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs:         map[string]any{"command": command},
		TargetCommands: allowedCommands,
	}
	return task
}

// makeTaskWithPaths constructs a task carrying the backend allowlists in the
// signed-task fields. Use makeTask (without this helper) to exercise the
// "backend did not send the field" branch — a nil slice.
func makeTaskWithPaths(command string, allowedCommands []string, allowedPaths []string) *types.Task {
	return makeTaskWithPathsAndMode(
		command,
		allowedCommands,
		allowedPaths,
		privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_UNSPECIFIED,
	)
}

func makeTaskWithPathsAndMode(
	command string,
	allowedCommands []string,
	allowedPaths []string,
	mode privateactionspb.RemoteActionAccessMode,
) *types.Task {
	task := makeTask(command, allowedCommands)
	task.Data.Attributes.TargetPaths = allowedPaths
	task.Data.Attributes.RemoteActionAccessMode = mode
	return task
}

func TestRemoteActionRemediationModeEnabled(t *testing.T) {
	cases := []struct {
		name string
		mode privateactionspb.RemoteActionAccessMode
		want bool
	}{
		{
			name: "unspecified",
			mode: privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_UNSPECIFIED,
			want: false,
		},
		{
			name: "read only",
			mode: privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_READ_ONLY,
			want: false,
		},
		{
			name: "read write",
			mode: privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_READ_WRITE,
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, remoteActionRemediationModeEnabled(tc.mode))
		})
	}
}

func TestRshellRunnerOptionsUseRemediationModeForReadWriteAccessMode(t *testing.T) {
	cases := []struct {
		name        string
		mode        privateactionspb.RemoteActionAccessMode
		wantContent string
		wantCode    int
	}{
		{
			name:        "unspecified keeps read only mode",
			mode:        privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_UNSPECIFIED,
			wantContent: "payload",
			wantCode:    2,
		},
		{
			name:        "read only keeps read only mode",
			mode:        privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_READ_ONLY,
			wantContent: "payload",
			wantCode:    2,
		},
		{
			name:        "read write enables remediation mode",
			mode:        privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_READ_WRITE,
			wantContent: "patched",
			wantCode:    0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "payload.txt")
			require.NoError(t, os.WriteFile(target, []byte("payload"), 0600))

			var stdout, stderr bytes.Buffer
			prog, err := interp.ParseScript("printf patched > payload.txt", "")
			require.NoError(t, err)

			runner, err := interp.New(rshellRunnerOptions(
				&stdout,
				&stderr,
				[]string{dir + ":rw"},
				[]string{"rshell:printf"},
				tc.mode,
			)...)
			require.NoError(t, err)
			defer runner.Close()

			runErr := runner.Run(context.Background(), prog)
			exitCode := 0
			if runErr != nil {
				var exitStatus interp.ExitStatus
				require.True(t, errors.As(runErr, &exitStatus), "unexpected run error: %v", runErr)
				exitCode = int(exitStatus)
			}

			content, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, exitCode)
			assert.Equal(t, tc.wantContent, string(content))
			assert.Empty(t, stdout.String())
			if tc.wantCode == 0 {
				assert.Empty(t, stderr.String())
			} else {
				assert.NotEmpty(t, stderr.String())
			}
		})
	}
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
			handler := NewRunCommandHandler(RunCommandHandlerConfig{})

			got := handler.filterAllowedCommands(tc.backend)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestFilterAllowedCommandsIntersectsConfiguredAgentAllowlist(t *testing.T) {
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
				AgentAllowedCommands:           tc.agent,
				AgentAllowedCommandsConfigured: true,
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
			name:    "backend paths are normalized in order",
			backend: []string{"/var/log", "/etc/"},
			want:    []string{"/var/log/", "/etc/"},
		},
		{
			name:    "backend access overlays are preserved",
			backend: []string{"/var/log:ro", "/var/log/datadog:rw"},
			want:    []string{"/var/log/:ro", "/var/log/datadog/:rw"},
		},
		{
			name:    "backend dot segments are cleaned without reducing siblings",
			backend: []string{"/var/./log/../log:ro", "/var/logger:rw"},
			want:    []string{"/var/log/:ro", "/var/logger/:rw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(RunCommandHandlerConfig{
				AgentAllowedPaths: []string{"/agent/path/that/should/be/ignored"},
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

func TestFilterAllowedPathsIntersectsConfiguredAgentAllowlistByAccess(t *testing.T) {
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
			name:     "backend descendant of agent path is not kept",
			agent:    []string{"/var/log:ro"},
			backend:  []string{"/var/log/datadog:ro"},
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
			name:     "read-only and read-write groups are combined in agent order",
			agent:    []string{"/var/log/datadog:ro", "/opt/datadog:rw", "/tmp/cache:ro"},
			backend:  []string{"/var/log:ro", "/opt:rw", "/tmp:rw"},
			expected: []string{"/var/log/datadog/:ro", "/opt/datadog/:rw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(RunCommandHandlerConfig{
				AgentAllowedPaths:           tc.agent,
				AgentAllowedPathsConfigured: true,
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
		AgentAllowedPaths:    paths,
		AgentAllowedCommands: commands,
	})

	assert.Equal(t, pathsCopy, paths, "AgentAllowedPaths input must not be mutated")
	assert.Equal(t, commandsCopy, commands, "AgentAllowedCommands input must not be mutated")
}

func TestRunCommandEmptyCommandReturnsError(t *testing.T) {
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

	_, err := handler.Run(context.Background(), makeTask("", nil), nil)

	assert.ErrorContains(t, err, "command is required")
}

func TestRunCommandNoAllowedCommandsBlocksExecution(t *testing.T) {
	// Backend nil → empty effective list → rshell rejects.
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

	out, err := handler.Run(context.Background(), makeTask("echo hello", nil), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandWithBackendAllowedCommand(t *testing.T) {
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

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
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

	out, err := handler.Run(context.Background(),
		makeTask("grep foo", []string{"rshell:echo"}), nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 127, result.ExitCode)
	assert.Contains(t, result.Stderr, "command not allowed")
}

func TestRunCommandConfiguredAgentCommandAllowlistNarrowsBackendPayload(t *testing.T) {
	handler := NewRunCommandHandler(RunCommandHandlerConfig{
		AgentAllowedCommands:           []string{"rshell:cat"},
		AgentAllowedCommandsConfigured: true,
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
		AgentAllowedCommands:           []string{},
		AgentAllowedCommandsConfigured: true,
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
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

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
	dir := t.TempDir()
	missing := "/__rshell_sandbox_warnings_test_missing__"
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

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
	dir := t.TempDir()
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

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
	dir := t.TempDir()
	handler := NewRunCommandHandler(RunCommandHandlerConfig{})

	task := makeTaskWithPathsAndMode("echo ok",
		[]string{"rshell:echo"},
		[]string{dir + ":rw"},
		privateactionspb.RemoteActionAccessMode_REMOTE_ACTION_ACCESS_MODE_READ_WRITE)

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
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "payload.txt"),
		bytes.Repeat([]byte("x"), 10*1024*1024+1),
		0600,
	))

	cases := []struct {
		name    string
		command string
		wantErr error
	}{
		{
			name:    "stdout limit",
			command: "cat payload.txt",
			wantErr: interp.ErrOutputLimitExceeded,
		},
		{
			name:    "stderr limit",
			command: "cat payload.txt >&2",
			wantErr: interp.ErrStderrLimitExceeded,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewRunCommandHandler(RunCommandHandlerConfig{})
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
	bundle := NewRshellBundle(&config.Config{})

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
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")

	handler := NewRunRemediationCommandHandler(
		[]string{setup.RShellPathAllowAll},
		[]string{setup.RShellCommandAllowAllWildcard},
	)
	task := makeTaskWithPaths("echo hello > "+target,
		[]string{"rshell:echo"},
		map[string][]string{setup.RShellPathAllowMapDefaultKey: {dir}})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.Equal(t, 0, result.ExitCode)
	content, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "hello\n", string(content))
}

// TestRunCommandReadOnlyBlocksFileRedirect verifies that the read-only
// runCommand handler rejects the same file-target redirection, leaving no
// file behind. This is the security guarantee that distinguishes the two
// actions.
func TestRunCommandReadOnlyBlocksFileRedirect(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")

	handler := NewRunCommandHandler(
		[]string{setup.RShellPathAllowAll},
		[]string{setup.RShellCommandAllowAllWildcard},
	)
	task := makeTaskWithPaths("echo hello > "+target,
		[]string{"rshell:echo"},
		map[string][]string{setup.RShellPathAllowMapDefaultKey: {dir}})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode,
		"read-only mode must reject file-target redirections")
	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr),
		"read-only mode must not create the redirect target")
}

// TestRunRemediationCommandRedirectOutsideSandboxBlocked verifies that even
// in remediation mode, a redirection target outside the effective AllowedPaths
// is rejected.
func TestRunRemediationCommandRedirectOutsideSandboxBlocked(t *testing.T) {
	allowedDir := t.TempDir()
	outsideTarget := filepath.Join(t.TempDir(), "out.txt")

	handler := NewRunRemediationCommandHandler(
		[]string{setup.RShellPathAllowAll},
		[]string{setup.RShellCommandAllowAllWildcard},
	)
	// Only allowedDir is in the sandbox; the write targets a sibling temp dir.
	task := makeTaskWithPaths("echo hello > "+outsideTarget,
		[]string{"rshell:echo"},
		map[string][]string{setup.RShellPathAllowMapDefaultKey: {allowedDir}})

	out, err := handler.Run(context.Background(), task, nil)

	require.NoError(t, err)
	result := out.(*RunCommandOutputs)
	assert.NotEqual(t, 0, result.ExitCode,
		"redirection outside the sandbox must be rejected even in remediation mode")
	_, statErr := os.Stat(outsideTarget)
	assert.True(t, os.IsNotExist(statErr))
}
