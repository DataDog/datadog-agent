// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{
		interp.StdIO(nil, &outBuf, &errBuf),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	}, opts...)

	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}

	err = runner.Run(context.Background(), prog)
	exitCode = 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestAllowedPathsOption(t *testing.T) {
	t.Run("nonexistent path skipped", func(t *testing.T) {
		runner, err := interp.New(
			interp.AllowedPaths([]string{"/nonexistent/path/that/does/not/exist"}),
		)
		require.NoError(t, err, "nonexistent paths should be skipped, not rejected")
		runner.Close()
	})

	t.Run("file path skipped", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0644))

		runner, err := interp.New(
			interp.AllowedPaths([]string{tmpFile}),
		)
		require.NoError(t, err, "file paths should be skipped (not directories)")
		runner.Close()
	})

	t.Run("valid directory accepted", func(t *testing.T) {
		dir := t.TempDir()
		runner, err := interp.New(
			interp.AllowedPaths([]string{dir}),
		)
		require.NoError(t, err)
		runner.Close()
	})
}

func TestDefaultDirFromAllowedPaths(t *testing.T) {
	t.Run("defaults to first allowed path when Dir is unset", func(t *testing.T) {
		first := t.TempDir()
		second := t.TempDir()
		runner, err := interp.New(
			interp.AllowedPaths([]string{first, second}),
		)
		require.NoError(t, err)
		defer runner.Close()
		assert.Equal(t, first, runner.Dir, "Dir should default to the first allowed path")
	})

	t.Run("falls back to os.Getwd when no allowed paths configured", func(t *testing.T) {
		runner, err := interp.New()
		require.NoError(t, err)
		defer runner.Close()
		cwd, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, cwd, runner.Dir, "Dir should fall back to os.Getwd() when no sandbox is configured")
	})

	t.Run("falls back to os.Getwd when all allowed paths are skipped", func(t *testing.T) {
		runner, err := interp.New(
			interp.AllowedPaths([]string{"/nonexistent/path/that/does/not/exist"}),
		)
		require.NoError(t, err)
		defer runner.Close()
		cwd, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, cwd, runner.Dir, "Dir should fall back to os.Getwd() when all paths are skipped")
	})
}

func TestAllowedPathsCatInside(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644))

	stdout, _, exitCode := runScript(t, "cat hello.txt", dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello world\n", stdout)
}

func TestAllowedPathsCatOutside(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "hidden.txt"), []byte("secret"), 0644))

	catPath := strings.ReplaceAll(filepath.Join(secret, "hidden.txt"), `\`, `/`)
	_, stderr, exitCode := runScript(t, "cat "+catPath, allowed,
		interp.AllowedPaths([]string{allowed}),
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

func TestAllowedPathsRedirectInside(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("redirected"), 0644))

	stdout, _, exitCode := runScript(t, "cat < data.txt", dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "redirected", stdout)
}

func TestAllowedPathsRedirectOutside(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "data.txt"), []byte("secret"), 0644))

	_, stderr, exitCode := runScript(t, "cat < "+shellQuoteForTest(filepath.Join(secret, "data.txt")), allowed,
		interp.AllowedPaths([]string{allowed}),
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

func TestAllowedPathsGlobInside(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte(""), 0644))

	stdout, _, exitCode := runScript(t, `echo *.txt`, dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "a.txt")
	assert.Contains(t, stdout, "b.txt")
}

func TestAllowedPathsAbsoluteGlobInside(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	require.NoError(t, os.Mkdir(data, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(data, "test-s-2-o"), []byte("2\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(data, "test-s-1-o"), []byte("1\n"), 0644))

	pattern := shellQuoteForTest(filepath.ToSlash(data)) + "/test-s-*-o"
	stdout, stderr, exitCode := runScript(t, "printf '%s\\n' "+pattern, dir,
		interp.AllowedPaths([]string{data}),
	)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stderr)
	assert.Equal(t, []string{
		filepath.Join(data, "test-s-1-o"),
		filepath.Join(data, "test-s-2-o"),
	}, cleanOutputPaths(stdout))
}

func TestAllowedPathsAbsoluteGlobOutsideStaysLiteral(t *testing.T) {
	dir := t.TempDir()
	allowed := filepath.Join(dir, "allowed")
	secret := filepath.Join(dir, "secret")
	require.NoError(t, os.Mkdir(allowed, 0755))
	require.NoError(t, os.Mkdir(secret, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(secret, "a.txt"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(secret, "b.txt"), []byte(""), 0644))

	pattern := shellQuoteForTest(filepath.ToSlash(secret)) + "/*.txt"
	stdout, stderr, exitCode := runScript(t, "printf '%s\\n' "+pattern, dir,
		interp.AllowedPaths([]string{allowed}),
	)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stderr)
	assert.Equal(t, []string{filepath.Join(secret, "*.txt")}, cleanOutputPaths(stdout))
}

func TestAllowedPathsGlobOutside(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "a.txt"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(secret, "b.txt"), []byte(""), 0644))

	// Glob on a directory outside allowed paths should return the literal pattern
	stdout, _, exitCode := runScript(t, `echo `+filepath.Join(secret, "*.txt"), allowed,
		interp.AllowedPaths([]string{allowed}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "*.txt") // pattern not expanded
}

func shellQuoteForTest(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func cleanOutputPaths(stdout string) []string {
	trimmed := strings.TrimSuffix(stdout, "\n")
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	for i, line := range lines {
		lines[i] = filepath.Clean(line)
	}
	return lines
}

func TestAllowedPathsTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	// Even if we try to traverse with .., os.Root should block it
	_, stderr, exitCode := runScript(t, `cat ../../etc/passwd`, dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

func TestAllowedPathsDoubleDotFilename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "..hidden"), []byte("dotdot content\n"), 0644))

	stdout, _, exitCode := runScript(t, `cat ..hidden`, dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "dotdot content\n", stdout)
}

func TestAllowedPathsPinsRootBeforeRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not applicable on Windows")
	}

	parent := t.TempDir()
	allowed := filepath.Join(parent, "allowed")
	secret := filepath.Join(parent, "secret")
	require.NoError(t, os.MkdirAll(allowed, 0755))
	require.NoError(t, os.MkdirAll(secret, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(allowed, "data.txt"), []byte("safe\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(secret, "data.txt"), []byte("secret\n"), 0644))

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("cat "+filepath.ToSlash(filepath.Join(allowed, "data.txt"))), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interp.AllowedPaths([]string{allowed}),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	require.NoError(t, err)
	defer runner.Close()

	movedAllowed := filepath.Join(parent, "allowed-moved")
	require.NoError(t, os.Rename(allowed, movedAllowed))
	require.NoError(t, os.Symlink(secret, allowed))

	err = runner.Run(context.Background(), prog)
	require.NoError(t, err)
	assert.Equal(t, "safe\n", outBuf.String())
	assert.Empty(t, errBuf.String())
}

func TestAllowedPathsEmptyBlocksAll(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644))

	_, stderr, exitCode := runScript(t, "cat test.txt", dir,
		interp.AllowedPaths([]string{}),
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

func TestAllowedPathsDefaultBlocksAll(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("works\n"), 0644))

	// No AllowedPaths option = blocks all file access
	_, stderr, exitCode := runScript(t, "cat test.txt", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

// TestParseScriptRejectsOversizedInput verifies that interp.ParseScript returns
// an error (without calling the parser) when the script byte length exceeds
// interp.MaxScriptBytes (5 MiB). This is the library-level enforcement that
// prevents callers from accidentally bypassing the limit by using the syntax
// parser directly.
func TestParseScriptRejectsOversizedInput(t *testing.T) {
	// A script just over the limit; syntactically valid so any error is from
	// the size check, not the parser.
	line := "echo hello\n"
	script := strings.Repeat(line, interp.MaxScriptBytes/len(line)+1)
	require.Greater(t, len(script), interp.MaxScriptBytes)

	_, err := interp.ParseScript(script, "test.sh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
	assert.Contains(t, err.Error(), "5 MiB")
}

// TestParseScriptAcceptsValidInput verifies that interp.ParseScript succeeds
// for a normal-sized, syntactically valid script.
func TestParseScriptAcceptsValidInput(t *testing.T) {
	prog, err := interp.ParseScript("echo hello\n", "test.sh")
	require.NoError(t, err)
	require.NotNil(t, prog)
}

// TestParseScriptRejectsInvalidSyntax verifies that interp.ParseScript
// propagates parse errors from the underlying syntax parser.
func TestParseScriptRejectsInvalidSyntax(t *testing.T) {
	_, err := interp.ParseScript(`echo "unterminated`, "test.sh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without closing quote")
}

func TestAllowedPathsClose(t *testing.T) {
	dir := t.TempDir()
	runner, err := interp.New(
		interp.AllowedPaths([]string{dir}),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	require.NoError(t, err)

	// Run once so Close is exercised after execution as well as before it.
	parser := syntax.NewParser()
	prog, _ := parser.Parse(strings.NewReader("true"), "")
	_ = runner.Run(context.Background(), prog)

	// Close should not panic, even if called twice
	require.NoError(t, runner.Close())
	require.NoError(t, runner.Close())
}

func TestSandboxWarningsDefaultGoToStderr(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interp.AllowedPaths([]string{"/nonexistent/path/that/does/not/exist"}),
	)
	require.NoError(t, err)
	defer runner.Close()

	assert.Contains(t, errBuf.String(), "AllowedPaths: skipping",
		"with no WarningsWriter option, warnings should fall back to stderr")
	assert.Empty(t, outBuf.String(), "warnings must never land on stdout")
}

func TestSandboxWarningsRoutedToDedicatedWriter(t *testing.T) {
	var outBuf, errBuf, warnBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interp.WarningsWriter(&warnBuf),
		interp.AllowedPaths([]string{"/nonexistent/path/that/does/not/exist"}),
	)
	require.NoError(t, err)
	defer runner.Close()

	assert.Contains(t, warnBuf.String(), "AllowedPaths: skipping",
		"warnings should be routed to the dedicated writer")
	assert.Empty(t, errBuf.String(),
		"with WarningsWriter set, stderr must remain clean of sandbox diagnostics")
	assert.Empty(t, outBuf.String(), "warnings must never land on stdout")
}

func TestSandboxWarningsAccessor(t *testing.T) {
	t.Run("returns warnings as slice", func(t *testing.T) {
		runner, err := interp.New(
			interp.WarningsWriter(io.Discard),
			interp.AllowedPaths([]string{
				"/nonexistent/path/one",
				"/nonexistent/path/two",
			}),
		)
		require.NoError(t, err)
		defer runner.Close()

		warnings := runner.Warnings()
		require.Len(t, warnings, 2, "one entry per skipped path")
		assert.Contains(t, warnings[0], "AllowedPaths: skipping")
		assert.Contains(t, warnings[1], "AllowedPaths: skipping")
	})

	t.Run("returns nil when no warnings emitted", func(t *testing.T) {
		dir := t.TempDir()
		runner, err := interp.New(
			interp.AllowedPaths([]string{dir}),
		)
		require.NoError(t, err)
		defer runner.Close()

		assert.Nil(t, runner.Warnings(),
			"a clean configuration should yield no warnings")
	})

	t.Run("accessor still works when streaming flush is suppressed", func(t *testing.T) {
		runner, err := interp.New(
			interp.WarningsWriter(io.Discard),
			interp.AllowedPaths([]string{"/nonexistent/path"}),
		)
		require.NoError(t, err)
		defer runner.Close()

		warnings := runner.Warnings()
		require.Len(t, warnings, 1,
			"io.Discard should suppress streaming output but not the accessor")
		assert.Contains(t, warnings[0], "AllowedPaths: skipping")
	})
}

func TestWarningsWriterRejectsNil(t *testing.T) {
	_, err := interp.New(interp.WarningsWriter(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writer must not be nil")
}
