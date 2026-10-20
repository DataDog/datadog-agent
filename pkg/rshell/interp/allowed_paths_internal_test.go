// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mvdan.cc/sh/v3/syntax"
)

func systemExecAllowedPaths(t *testing.T) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return []string{filepath.Join(os.Getenv("SystemRoot"), "System32")}
	}
	return []string{"/bin", "/usr"}
}

func runScriptInternal(t *testing.T, script, dir string, opts ...RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]RunnerOption{
		StdIO(nil, &outBuf, &errBuf),
		allowAllCommandsOpt(),
	}, opts...)

	runner, err := New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}
	runner.execHandler = func(ctx context.Context, args []string) error {
		hc := HandlerCtx(ctx)
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = hc.Dir
		cmd.Stdout = hc.Stdout
		cmd.Stderr = hc.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return ExitStatus(exitErr.ExitCode())
			}
			return err
		}
		return nil
	}

	err = runner.Run(context.Background(), prog)
	exitCode = 0
	if err != nil {
		var es ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestAllowedPathsExecBlocked(t *testing.T) {
	dir := t.TempDir()
	// Exec is always blocked when AllowedPaths is set, even for commands inside allowed paths
	_, stderr, exitCode := runScriptInternal(t, `/bin/echo hello`, dir,
		AllowedPaths(append([]string{dir}, systemExecAllowedPaths(t)...)),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "unknown command")
}

func TestAllowedPathsExecNonexistent(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := runScriptInternal(t, `totally_nonexistent_cmd_12345`, dir,
		AllowedPaths(append([]string{dir}, systemExecAllowedPaths(t)...)),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "unknown command")
}

func TestAllowedPathsExecViaPathLookup(t *testing.T) {
	dir := t.TempDir()
	// "date" exists on PATH but /bin and /usr are not in AllowedPaths.
	// The default noExecHandler must reject it. We avoid runScriptInternal
	// because it overrides execHandler with a real exec.Command, bypassing
	// the sandbox. We also cannot use a builtin name (find, grep, sed, etc.)
	// because builtins are resolved before the exec handler is consulted.
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("date"), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	runner, err := New(
		StdIO(nil, &outBuf, &errBuf),
		AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	defer runner.Close()
	runner.Dir = dir

	err = runner.Run(context.Background(), prog)
	exitCode := 0
	if err != nil {
		var es ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, errBuf.String(), "command not allowed")
}

func TestAllowedPathsExecSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not applicable on Windows")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))

	// Create a symlink inside the allowed dir pointing to /bin/echo outside it.
	require.NoError(t, os.Symlink("/bin/echo", filepath.Join(binDir, "escape_echo")))

	// Only allow the temp dir — the symlink target (/bin/echo) is outside.
	_, stderr, exitCode := runScriptInternal(t, filepath.Join(binDir, "escape_echo")+" hello", dir,
		AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "unknown command")
}

func TestRunRecoversPanic(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	runner, err := New(StdIO(nil, &outBuf, &errBuf), allowAllCommandsOpt())
	require.NoError(t, err)
	defer runner.Close()

	// Trigger initial reset so we can override the exec handler.
	runner.Reset()

	// Install an exec handler that panics.
	runner.execHandler = func(ctx context.Context, args []string) error {
		panic("deliberate test panic")
	}

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("somecmd"), "")
	require.NoError(t, err)

	err = runner.Run(context.Background(), prog)
	require.Error(t, err)
	// The error returned to the caller must be generic — it must not include
	// the panic value to avoid leaking internal state to untrusted callers.
	assert.Equal(t, "internal error", err.Error())
	assert.NotContains(t, err.Error(), "deliberate test panic")
	// Panic details are written to the runner's stderr (not os.Stderr), so
	// they stay within the configured I/O boundary.
	assert.Contains(t, errBuf.String(), "deliberate test panic")
}

func TestRunZeroValueRunnerReturnsError(t *testing.T) {
	// A zero-value Runner (not created via New) should return an explicit
	// error from Run instead of panicking.
	var r Runner
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("echo hi"), "")
	require.NoError(t, err)

	err = r.Run(context.Background(), prog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use interp.New to construct a Runner")
}

func TestAllowedPathsExecDefaultBlocksAll(t *testing.T) {
	dir := t.TempDir()
	// No AllowedPaths option — default noExecHandler blocks all external commands.
	// With AllowAllCommands (set by runScriptInternal), the command reaches the
	// exec handler which returns "unknown command" via noExecHandler.
	_, stderr, exitCode := runScriptInternal(t, `/bin/echo hello`, dir)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "unknown command")
}

// TestHostPrefixAfterAllowedPaths verifies that HostPrefix applied after
// AllowedPaths correctly sets the sandbox's host prefix.
func TestHostPrefixAfterAllowedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("container host prefix is not supported on Windows")
	}
	dir := t.TempDir()
	runner, err := New(
		AllowedPaths([]string{dir}),
		HostPrefix("/custom"),
	)
	require.NoError(t, err)
	defer runner.Close()

	assert.Equal(t, "/custom", runner.sandbox.HostPrefix())
}

// TestHostPrefixBeforeAllowedPaths verifies that HostPrefix applied before
// AllowedPaths still correctly sets the sandbox's host prefix.
func TestHostPrefixBeforeAllowedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("container host prefix is not supported on Windows")
	}
	dir := t.TempDir()
	runner, err := New(
		HostPrefix("/custom"),
		AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	defer runner.Close()

	assert.Equal(t, "/custom", runner.sandbox.HostPrefix())
}

// TestHostPrefixWithoutAllowedPaths verifies that HostPrefix is silently
// ignored when no AllowedPaths is configured (sandbox is nil).
func TestHostPrefixWithoutAllowedPaths(t *testing.T) {
	runner, err := New(
		HostPrefix("/custom"),
	)
	require.NoError(t, err)
	defer runner.Close()

	assert.Nil(t, runner.sandbox, "sandbox should be nil when AllowedPaths is not set")
}

// TestHostPrefixDefaultWhenNotSet verifies that the sandbox has no host
// prefix when HostPrefix is not called (container resolution disabled).
func TestHostPrefixDefaultWhenNotSet(t *testing.T) {
	dir := t.TempDir()
	runner, err := New(
		AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	defer runner.Close()

	assert.Empty(t, runner.sandbox.HostPrefix())
}

// TestAllowedPathsEnvVar verifies that ALLOWED_PATHS is set in the
// interpreter's environment with the resolved absolute paths.
func TestAllowedPathsEnvVar(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	stdout, _, _ := runScriptInternal(t, `echo $ALLOWED_PATHS`, dir1,
		AllowedPaths([]string{dir1, dir2}),
	)

	expected := dir1 + string(filepath.ListSeparator) + dir2
	assert.Equal(t, expected+"\n", stdout)
}

// TestAllowedPathsEnvVarSinglePath verifies ALLOWED_PATHS with one path.
func TestAllowedPathsEnvVarSinglePath(t *testing.T) {
	dir := t.TempDir()

	stdout, _, _ := runScriptInternal(t, `echo $ALLOWED_PATHS`, dir,
		AllowedPaths([]string{dir}),
	)

	assert.Equal(t, dir+"\n", stdout)
}

// TestAllowedPathsEnvVarManyDirs verifies ALLOWED_PATHS with several directories.
func TestAllowedPathsEnvVarManyDirs(t *testing.T) {
	dirs := make([]string, 5)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}

	stdout, _, _ := runScriptInternal(t, `echo $ALLOWED_PATHS`, dirs[0],
		AllowedPaths(dirs),
	)

	expected := strings.Join(dirs, string(filepath.ListSeparator))
	assert.Equal(t, expected+"\n", stdout)
}

// TestAllowedPathsEnvVarNestedDirs verifies ALLOWED_PATHS with deeply
// nested directories.
func TestAllowedPathsEnvVarNestedDirs(t *testing.T) {
	root := t.TempDir()
	nested1 := filepath.Join(root, "a", "b", "c")
	nested2 := filepath.Join(root, "x", "y")
	require.NoError(t, os.MkdirAll(nested1, 0755))
	require.NoError(t, os.MkdirAll(nested2, 0755))

	stdout, _, _ := runScriptInternal(t, `echo $ALLOWED_PATHS`, root,
		AllowedPaths([]string{nested1, nested2}),
	)

	expected := nested1 + string(filepath.ListSeparator) + nested2
	assert.Equal(t, expected+"\n", stdout)
}

// TestAllowedPathsEnvVarParentAndChild verifies ALLOWED_PATHS when both
// a parent and child directory are allowed.
func TestAllowedPathsEnvVarParentAndChild(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "sub", "dir")
	require.NoError(t, os.MkdirAll(child, 0755))

	stdout, _, _ := runScriptInternal(t, `echo $ALLOWED_PATHS`, root,
		AllowedPaths([]string{root, child}),
	)

	expected := root + string(filepath.ListSeparator) + child
	assert.Equal(t, expected+"\n", stdout)
}

// TestAllowedPathsEnvVarSkipsNonexistent verifies that ALLOWED_PATHS only
// contains directories that were successfully opened.
func TestAllowedPathsEnvVarSkipsNonexistent(t *testing.T) {
	dir := t.TempDir()

	stdout, _, _ := runScriptInternal(t, `echo $ALLOWED_PATHS`, dir,
		AllowedPaths([]string{"/nonexistent/path", dir}),
	)

	assert.Equal(t, dir+"\n", stdout)
}

// TestAllowedPathsEnvVarNotSetWithoutSandbox verifies that ALLOWED_PATHS
// is not set when AllowedPaths is not configured.
func TestAllowedPathsEnvVarNotSetWithoutSandbox(t *testing.T) {
	dir := t.TempDir()

	runner, err := New()
	require.NoError(t, err)
	defer runner.Close()

	runner.Dir = dir
	v := runner.Env.Get("ALLOWED_PATHS")
	assert.False(t, v.IsSet(), "ALLOWED_PATHS should not be set without AllowedPaths")
}
