// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package lsof_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestLsofHappyPathShowsPathWithinAllowedPaths verifies that lsof -p <pid>
// lists a real open file's path in the NAME column when that path falls
// within an AllowedPaths root. The pid used is the test binary's own pid:
// rshell builtins run in-process, so os.Getpid() here is exactly the pid
// whose /proc/<pid>/fd the lsof builtin will read.
func TestLsofHappyPathShowsPathWithinAllowedPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	script := fmt.Sprintf("lsof -p %d", os.Getpid())
	stdout, stderr, code := cmdRun(t, script, []string{dir})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, path) {
		t.Errorf("stdout does not contain open file path %q:\n%s", path, stdout)
	}
	if !strings.Contains(stdout, "REG") {
		t.Errorf("stdout does not contain TYPE REG:\n%s", stdout)
	}
	// Other fds of the test binary (cwd, root, txt, stdin, ...) legitimately
	// fall outside dir and are correctly redacted; only the row for the file
	// this test created must be unrestricted.
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, path) && strings.Contains(line, "(restricted)") {
			t.Errorf("row for %q unexpectedly shows (restricted) even though it is inside AllowedPaths:\n%s", path, stdout)
		}
	}
}

// TestLsofGatingRedactsPathOutsideAllowedPaths verifies that the same open
// file's path is replaced with "(restricted)" when its directory is not
// among the configured AllowedPaths roots — the deliberate divergence from
// ss/df documented in builtins/lsof's package doc comment.
func TestLsofGatingRedactsPathOutsideAllowedPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	otherDir := t.TempDir()

	script := fmt.Sprintf("lsof -p %d", os.Getpid())
	stdout, stderr, code := cmdRun(t, script, []string{otherDir})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, path) {
		t.Errorf("stdout leaked the real path outside AllowedPaths:\n%s", stdout)
	}
	if !strings.Contains(stdout, "(restricted)") {
		t.Errorf("stdout does not contain (restricted):\n%s", stdout)
	}
}

// TestLsofGatingRedactsSizeDeviceNodeOutsideAllowedPaths verifies that a
// restricted row hides not only NAME but also DEVICE/SIZE/NODE: those are
// per-file attributes of the exact same out-of-sandbox path, so an agent
// that cannot see where a file lives must also not be able to learn its
// exact byte count, device number, or inode (e.g. to fingerprint a specific
// restricted file such as /etc/shadow).
func TestLsofGatingRedactsSizeDeviceNodeOutsideAllowedPaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	// A distinctive, exact size so its presence/absence in stdout is
	// unambiguous (unlike "0" or another value that could collide with an
	// unrelated column, e.g. a PID).
	const distinctiveSize = 424242
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), distinctiveSize), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	otherDir := t.TempDir()

	script := fmt.Sprintf("lsof -p %d", os.Getpid())
	stdout, stderr, code := cmdRun(t, script, []string{otherDir})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, strconv.Itoa(distinctiveSize)) {
		t.Errorf("stdout leaked the restricted file's real SIZE:\n%s", stdout)
	}
}

// TestLsofGatingRedactsPathWithNoAllowedPathsConfigured verifies the
// documented default: an empty AllowedPaths list means no filesystem paths
// are reachable, so every NAME is redacted, not shown unrestricted.
func TestLsofGatingRedactsPathWithNoAllowedPathsConfigured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	script := fmt.Sprintf("lsof -p %d", os.Getpid())
	stdout, stderr, code := cmdRun(t, script, nil)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, path) {
		t.Errorf("stdout leaked the real path with no AllowedPaths configured:\n%s", stdout)
	}
	if !strings.Contains(stdout, "(restricted)") {
		t.Errorf("stdout does not contain (restricted):\n%s", stdout)
	}
}

// TestLsofDeletedFileMarker verifies that a file removed while still held
// open is reported with a " (deleted)" suffix on NAME — the primary
// diagnostic this builtin exists for.
func TestLsofDeletedFileMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	script := fmt.Sprintf("lsof -p %d", os.Getpid())
	stdout, stderr, code := cmdRun(t, script, []string{dir})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, path+" (deleted)") {
		t.Errorf("stdout does not contain %q:\n%s", path+" (deleted)", stdout)
	}
}

// TestLsofDeletedFileRestrictedMarker verifies that a deleted-open file
// outside AllowedPaths still surfaces the deleted-file diagnostic signal
// without leaking the path: "(restricted) (deleted)".
func TestLsofDeletedFileRestrictedMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	otherDir := t.TempDir()
	script := fmt.Sprintf("lsof -p %d", os.Getpid())
	stdout, stderr, code := cmdRun(t, script, []string{otherDir})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, path) {
		t.Errorf("stdout leaked the real path outside AllowedPaths:\n%s", stdout)
	}
	if !strings.Contains(stdout, "(restricted) (deleted)") {
		t.Errorf("stdout does not contain \"(restricted) (deleted)\":\n%s", stdout)
	}
}

// TestLsofSelectorNoMatchExitsNonzero verifies that a selector matching no
// open files exits 1, matching real lsof's "nothing matched" behaviour.
func TestLsofSelectorNoMatchExitsNonzero(t *testing.T) {
	stdout, _, code := cmdRun(t, "lsof -c this-command-name-should-never-exist-anywhere", nil)
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stdout: %s", code, stdout)
	}
}
