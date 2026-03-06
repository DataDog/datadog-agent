// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

func runScriptInternal(t *testing.T, script, dir string, opts ...RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]RunnerOption{
		StdIO(nil, &outBuf, &errBuf),
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

func TestAllowedPathsExecInside(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		// On Windows, use the system directory for echo
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		stdout, _, exitCode := runScriptInternal(t, `echo hello`, dir,
			AllowedPaths([]string{dir, systemRoot}),
		)
		assert.Equal(t, 0, exitCode)
		assert.Equal(t, "hello\n", stdout)
	} else {
		// /bin/echo should be within the allowed path if we allow /bin or /usr
		stdout, _, exitCode := runScriptInternal(t, `/bin/echo hello`, dir,
			AllowedPaths([]string{dir, "/bin", "/usr"}),
		)
		assert.Equal(t, 0, exitCode)
		assert.Equal(t, "hello\n", stdout)
	}
}

func TestAllowedPathsExecOutside(t *testing.T) {
	dir := t.TempDir()
	// Only allow the temp dir, so /bin/echo should be blocked
	_, stderr, exitCode := runScriptInternal(t, `/bin/echo hello`, dir,
		AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "command not found")
}

func TestAllowedPathsExecNonexistent(t *testing.T) {
	dir := t.TempDir()
	allowedPaths := []string{dir, "/bin", "/usr"}
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		allowedPaths = []string{dir, systemRoot}
	}
	// Command that doesn't exist at all — ExecLookPathDir fails
	_, stderr, exitCode := runScriptInternal(t, `totally_nonexistent_cmd_12345`, dir,
		AllowedPaths(allowedPaths),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "command not found")
}

func TestAllowedPathsExecViaPathLookup(t *testing.T) {
	dir := t.TempDir()
	// "ls" is resolved via PATH (not absolute), but /bin and /usr are not allowed
	_, stderr, exitCode := runScriptInternal(t, `ls`, dir,
		AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "command not found")
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
	assert.Contains(t, stderr, "command not found")
}

func TestAllowedPathsExecDefaultBlocksAll(t *testing.T) {
	dir := t.TempDir()
	// No AllowedPaths option — default blocks all exec
	_, stderr, exitCode := runScriptInternal(t, `/bin/echo hello`, dir)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "command not found")
}
