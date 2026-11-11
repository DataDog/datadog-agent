// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package head_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func TestHeadSymlinkToFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "real.txt", "hello from real\n")
	// Use a relative symlink target so os.Root can follow it within the sandbox.
	require.NoError(t, os.Symlink("real.txt", filepath.Join(dir, "link.txt")))
	stdout, _, code := cmdRun(t, "head -n 1 link.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello from real\n", stdout)
}

func TestHeadDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	// Create a symlink pointing nowhere.
	require.NoError(t, os.Symlink(filepath.Join(dir, "does_not_exist.txt"), filepath.Join(dir, "dangling.txt")))
	_, stderr, code := cmdRun(t, "head dangling.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadDevNull(t *testing.T) {
	// /dev/null is an empty source: head should output nothing and exit 0.
	// (Only meaningful on Unix; uses allowed path bypass since /dev/null is outside dir.)
	dir := t.TempDir()
	stdout, _, code := runScript(t, "head /dev/null", dir,
		interp.AllowedPaths([]string{dir, "/dev"}),
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestHeadPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		// The original executable and TMPDIR may be inside root-only directories.
		dir, err := os.MkdirTemp("/tmp", "rshell-head-permission-")
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, os.RemoveAll(dir)) })

		executable, err := os.Executable()
		require.NoError(t, err)
		binary, err := os.ReadFile(executable)
		require.NoError(t, err)
		childExecutable := filepath.Join(dir, "head.test")
		require.NoError(t, os.WriteFile(childExecutable, binary, 0o755))
		require.NoError(t, os.Chmod(childExecutable, 0o755))
		// Hand the directory over only after root's writes, so an unprivileged
		// process cannot plant a symlink for root to write through.
		require.NoError(t, os.Chown(dir, 65534, 65534))

		command := exec.Command(childExecutable, "-test.run=^TestHeadPermissionDenied$", "-test.v")
		command.Dir = dir
		// Don't inherit the test runner's environment: under Bazel it points the
		// child at root-owned coverage, XML report and runfiles paths.
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + dir}
		command.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: 65534, Gid: 65534},
		}
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
		t.Logf("unprivileged child:\n%s", output)
		return
	}

	dir := t.TempDir()
	writeFile(t, dir, "noperms.txt", "secret\n")
	stdout, stderr, code := cmdRun(t, "head noperms.txt", dir)
	require.Equal(t, 0, code)
	require.Equal(t, "secret\n", stdout)
	require.Empty(t, stderr)
	require.NoError(t, os.Chmod(filepath.Join(dir, "noperms.txt"), 0000))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(dir, "noperms.txt"), 0644)
	})
	stdout, stderr, code = cmdRun(t, "head noperms.txt", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "head:")
	assert.Contains(t, stderr, "permission denied")
}
