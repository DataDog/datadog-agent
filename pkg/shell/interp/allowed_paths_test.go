// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package interp_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{
		interp.StdIO(nil, &outBuf, &errBuf),
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
	t.Run("invalid path rejected", func(t *testing.T) {
		_, err := interp.New(
			interp.AllowedPaths([]string{"/nonexistent/path/that/does/not/exist"}),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AllowedPaths")
	})

	t.Run("file not directory rejected", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0644))

		_, err := interp.New(
			interp.AllowedPaths([]string{tmpFile}),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
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

	_, stderr, exitCode := runScript(t, "cat "+filepath.Join(secret, "hidden.txt"), allowed,
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

	_, stderr, exitCode := runScript(t, "cat < "+filepath.Join(secret, "data.txt"), allowed,
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

func TestAllowedPathsTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	// Even if we try to traverse with .., os.Root should block it
	_, stderr, exitCode := runScript(t, `cat ../../etc/passwd`, dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
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

func TestAllowedPathsNilUnrestricted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("works\n"), 0644))

	// No AllowedPaths option = nil = unrestricted
	stdout, _, exitCode := runScript(t, "cat test.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "works\n", stdout)
}


func TestAllowedPathsClose(t *testing.T) {
	dir := t.TempDir()
	runner, err := interp.New(
		interp.AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)

	// Trigger Reset to open roots
	parser := syntax.NewParser()
	prog, _ := parser.Parse(strings.NewReader("true"), "")
	_ = runner.Run(context.Background(), prog)

	// Close should not panic, even if called twice
	require.NoError(t, runner.Close())
	require.NoError(t, runner.Close())
}
