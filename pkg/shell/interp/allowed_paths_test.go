// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

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

func runScript(t *testing.T, script string, opts ...interp.RunnerOption) (stdout, stderr string, exitCode int) {
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

func TestAllowedPaths_CatAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644))

	stdout, _, exitCode := runScript(t, `cat hello.txt`,
		interp.Dir(dir),
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello\n", stdout)
}

func TestAllowedPaths_CatBlocked(t *testing.T) {
	allowed := t.TempDir()
	blocked := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "secret.txt"), []byte("secret\n"), 0644))

	_, stderr, exitCode := runScript(t,
		`cat `+filepath.Join(blocked, "secret.txt"),
		interp.Dir(allowed),
		interp.AllowedPaths([]string{allowed}),
	)
	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "outside allowed directories")
}

func TestAllowedPaths_RedirectAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input.txt"), []byte("from redirect\n"), 0644))

	stdout, _, exitCode := runScript(t, `cat < input.txt`,
		interp.Dir(dir),
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "from redirect\n", stdout)
}

func TestAllowedPaths_RedirectBlocked(t *testing.T) {
	allowed := t.TempDir()
	blocked := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "input.txt"), []byte("secret\n"), 0644))

	_, stderr, exitCode := runScript(t,
		`cat < `+filepath.Join(blocked, "input.txt"),
		interp.Dir(allowed),
		interp.AllowedPaths([]string{allowed}),
	)
	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "outside allowed directories")
}

func TestAllowedPaths_MultipleRoots(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("aaa\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("bbb\n"), 0644))

	stdout, _, exitCode := runScript(t,
		`cat `+filepath.Join(dir1, "a.txt")+"\n"+`cat `+filepath.Join(dir2, "b.txt"),
		interp.Dir(dir1),
		interp.AllowedPaths([]string{dir1, dir2}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "aaa\nbbb\n", stdout)
}

func TestAllowedPaths_SymlinkEscape(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret\n"), 0644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(allowed, "link.txt")))

	_, stderr, exitCode := runScript(t, `cat link.txt`,
		interp.Dir(allowed),
		interp.AllowedPaths([]string{allowed}),
	)
	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "link.txt")
}

func TestAllowedPaths_DotDotTraversal(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret\n"), 0644))

	_, stderr, exitCode := runScript(t, `cat ../secret.txt`,
		interp.Dir(child),
		interp.AllowedPaths([]string{child}),
	)
	assert.NotEqual(t, 0, exitCode)
	assert.Contains(t, stderr, "outside allowed directories")
}

func TestAllowedPaths_SubdirAllowed(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "file.txt"), []byte("nested\n"), 0644))

	stdout, _, exitCode := runScript(t, `cat sub/file.txt`,
		interp.Dir(dir),
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "nested\n", stdout)
}

func TestAllowedPaths_NoRestriction(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644))

	stdout, _, exitCode := runScript(t, `cat hello.txt`,
		interp.Dir(dir),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello\n", stdout)
}

func TestAllowedPaths_InvalidPath(t *testing.T) {
	_, err := interp.New(
		interp.AllowedPaths([]string{"/nonexistent/path/that/does/not/exist"}),
	)
	assert.Error(t, err)
}

func TestAllowedPaths_EmptySliceNoRestriction(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644))

	stdout, _, exitCode := runScript(t, `cat hello.txt`,
		interp.Dir(dir),
		interp.AllowedPaths([]string{}),
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello\n", stdout)
}
