// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
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
	var script string
	var allowed []string
	if runtime.GOOS == "windows" {
		script = `C:\Windows\System32\cmd.exe /c echo hello`
		allowed = []string{dir, `C:\Windows\System32`}
	} else {
		script = `/bin/echo hello`
		allowed = []string{dir, "/bin", "/usr"}
	}
	stdout, _, exitCode := runScriptInternal(t, script, dir,
		AllowedPaths(allowed),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello\n", stdout)
}

func TestAllowedPathsExecOutside(t *testing.T) {
	dir := t.TempDir()
	// Only allow the temp dir, so the system command should be blocked
	var script string
	if runtime.GOOS == "windows" {
		script = `C:\Windows\System32\cmd.exe /c echo hello`
	} else {
		script = `/bin/echo hello`
	}
	_, stderr, exitCode := runScriptInternal(t, script, dir,
		AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "not found")
}

func TestAllowedPathsExecNonexistent(t *testing.T) {
	dir := t.TempDir()
	// Command that doesn't exist at all — LookPathDir fails
	allowed := []string{dir}
	if runtime.GOOS == "windows" {
		allowed = append(allowed, `C:\Windows\System32`)
	} else {
		allowed = append(allowed, "/bin", "/usr")
	}
	_, stderr, exitCode := runScriptInternal(t, `totally_nonexistent_cmd_12345`, dir,
		AllowedPaths(allowed),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "not found")
}

func TestAllowedPathsExecViaPathLookup(t *testing.T) {
	dir := t.TempDir()
	// "ls" is resolved via PATH (not absolute), but /bin and /usr are not allowed
	_, stderr, exitCode := runScriptInternal(t, `ls`, dir,
		AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "not found")
}

func TestAllowedPathsExecDefaultBlocksAll(t *testing.T) {
	dir := t.TempDir()
	// No AllowedPaths option — default blocks all exec
	var script string
	if runtime.GOOS == "windows" {
		script = `C:\Windows\System32\cmd.exe /c echo hello`
	} else {
		script = `/bin/echo hello`
	}
	_, stderr, exitCode := runScriptInternal(t, script, dir)
	assert.Equal(t, 127, exitCode)
	assert.Contains(t, stderr, "not found")
}
