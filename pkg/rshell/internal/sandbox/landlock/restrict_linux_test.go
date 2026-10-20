// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package landlock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	ll "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/stretchr/testify/require"
)

func TestAccessMappings(t *testing.T) {
	require.Equal(t, uint64(ll.AccessFSReadFile|ll.AccessFSReadDir), readAccess)
	require.Equal(t, uint64(
		ll.AccessFSReadFile|
			ll.AccessFSReadDir|
			ll.AccessFSWriteFile|
			ll.AccessFSTruncate|
			ll.AccessFSMakeReg,
	), readWriteAccess)
	require.Equal(t, uint64((1<<15)-1), handledAccess)
	require.NotZero(t, handledAccess&uint64(ll.AccessFSExecute))
	require.Zero(t, readWriteAccess&uint64(ll.AccessFSExecute))
	require.Zero(t, readWriteAccess&uint64(ll.AccessFSRemoveFile))
}

func TestRestrictEnforcesReadAndWritePolicy(t *testing.T) {
	if raceEnabled {
		t.Skip("the race runtime uses libpsx, which needs /proc after Landlock is installed")
	}
	if os.Getenv("RSHELL_LANDLOCK_TEST_HELPER") == "1" {
		runRestrictHelper()
		return
	}

	root := t.TempDir()
	readOnly := filepath.Join(root, "read-only")
	readWrite := filepath.Join(root, "read-write")
	outside := filepath.Join(root, "outside")
	removable := filepath.Join(root, "removable")
	for _, path := range []string{readOnly, readWrite, outside, removable} {
		require.NoError(t, os.Mkdir(path, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(path, "existing"), []byte("data"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(path, "remove"), []byte("data"), 0o600))
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRestrictEnforcesReadAndWritePolicy$")
	cmd.Env = append(os.Environ(),
		"RSHELL_LANDLOCK_TEST_HELPER=1",
		"RSHELL_LANDLOCK_TEST_RO="+readOnly,
		"RSHELL_LANDLOCK_TEST_RW="+readWrite,
		"RSHELL_LANDLOCK_TEST_OUTSIDE="+outside,
		"RSHELL_LANDLOCK_TEST_REMOVABLE="+removable,
	)
	runLandlockSubprocess(t, cmd)
}

func runRestrictHelper() {
	readOnly := os.Getenv("RSHELL_LANDLOCK_TEST_RO")
	readWrite := os.Getenv("RSHELL_LANDLOCK_TEST_RW")
	outside := os.Getenv("RSHELL_LANDLOCK_TEST_OUTSIDE")
	removable := os.Getenv("RSHELL_LANDLOCK_TEST_REMOVABLE")
	trustedFile := filepath.Join(outside, "existing")
	missingOptional := filepath.Join(outside, "missing-directory")
	mustRestrict([]string{readOnly + ":ro", readWrite + ":rw"}, []TrustedPath{
		{Path: trustedFile, Kind: TrustedPathFile, Access: TrustedPathReadOnly},
		{Path: removable, Kind: TrustedPathDirectory, Access: TrustedPathReadRemoveFiles},
		{Path: missingOptional, Kind: TrustedPathDirectory, Access: TrustedPathReadOnly, Optional: true},
	})

	mustRead(filepath.Join(readOnly, "existing"))
	mustRead(filepath.Join(readWrite, "existing"))
	mustRead(trustedFile)
	mustDenyRead(filepath.Join(outside, "remove"))
	mustDenyWrite(filepath.Join(readOnly, "new"))
	mustDenyWrite(filepath.Join(readOnly, "existing"))
	mustWrite(filepath.Join(readWrite, "new"))
	mustWrite(filepath.Join(readWrite, "existing"))
	mustDenyWrite(filepath.Join(outside, "new"))
	mustDenyRemove(filepath.Join(readOnly, "remove"))
	mustDenyRemove(filepath.Join(readWrite, "remove"))
	mustRemove(filepath.Join(removable, "remove"))
	mustDenyWrite(filepath.Join(removable, "new"))
	mustDenyMkdir(filepath.Join(readWrite, "directory"))
	mustDenySymlink(filepath.Join(readWrite, "symlink"))
	mustDenyExecute(os.Args[0])
	mustUseDevNull()
}

func TestOpenRulesFailsClosedForMissingRequiredPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := openRules([]string{missing + ":rw"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open Landlock path")
}

func TestOpenRulesSupportsExactFileAndOptionalDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "exact-file")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))

	rules, err := openRules(nil, []TrustedPath{
		{Path: file, Kind: TrustedPathFile, Access: TrustedPathReadOnly},
		{Path: filepath.Join(root, "missing"), Kind: TrustedPathDirectory, Optional: true},
	})
	require.NoError(t, err)
	defer closeOpenedRules(rules)
	require.Len(t, rules, 2)
	require.Equal(t, file, rules[0].path)
	require.Equal(t, uint64(ll.AccessFSReadFile), rules[0].allowedAccess)
	require.Equal(t, "/dev/null", rules[1].path)
	require.Equal(t, devNullAccess, rules[1].allowedAccess)
}

func TestOpenRulesRejectsRemoveGrantForExactFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("data"), 0o600))

	_, err := openRules(nil, []TrustedPath{{
		Path: file, Kind: TrustedPathFile, Access: TrustedPathReadRemoveFiles,
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot remove files")
}

func TestOpenRulesAlwaysGrantsExactDevNullReadWrite(t *testing.T) {
	rules, err := openRules(nil, nil)
	require.NoError(t, err)
	defer closeOpenedRules(rules)
	require.Len(t, rules, 1)
	require.Equal(t, "/dev/null", rules[0].path)
	require.Equal(t, devNullAccess, rules[0].allowedAccess)
}

func TestOpenRulesRejectsResolvedReadOnlyChildOfReadWriteRoot(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o700))

	_, err := openRules([]string{parent + ":rw", child + ":ro"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot represent")
}

func TestOpenRulesReadOnlyDowngradesEveryAllowedPath(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o700))

	rules, err := openRulesReadOnly([]string{parent + ":rw", child + ":ro"}, nil)
	require.NoError(t, err)
	defer closeOpenedRules(rules)
	require.Len(t, rules, 3)
	for _, rule := range rules[:2] {
		require.Equal(t, accessReadOnly, rule.mode)
		require.Equal(t, readAccess, rule.allowedAccess)
	}
}

func TestOpenRulesRejectsBackendSymlinkRoot(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o700))
	alias := filepath.Join(t.TempDir(), "child-alias")
	require.NoError(t, os.Symlink(child, alias))

	_, err := openRules([]string{alias + ":ro"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open Landlock path")
}

func TestOpenRulesAcceptsDirectBackendRoot(t *testing.T) {
	direct := t.TempDir()

	rules, err := openRules([]string{direct + ":ro"}, nil)
	require.NoError(t, err)
	closeOpenedRules(rules)
}

func TestOpenRulesAllowsResolvedReadWriteChildOfReadOnlyRoot(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o700))

	rules, err := openRules([]string{parent + ":ro", child + ":rw"}, nil)
	require.NoError(t, err)
	closeOpenedRules(rules)
}

func TestRestrictUsesValidatedDescriptorAfterPathReplacement(t *testing.T) {
	if raceEnabled {
		t.Skip("the race runtime uses libpsx, which needs /proc after Landlock is installed")
	}
	if os.Getenv("RSHELL_LANDLOCK_FD_TEST_HELPER") == "1" {
		runDescriptorIdentityHelper()
		return
	}

	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(allowed, "existing"), []byte("original"), 0o600))

	cmd := exec.Command(os.Args[0], "-test.run=^TestRestrictUsesValidatedDescriptorAfterPathReplacement$")
	cmd.Env = append(os.Environ(),
		"RSHELL_LANDLOCK_FD_TEST_HELPER=1",
		"RSHELL_LANDLOCK_FD_TEST_ALLOWED="+allowed,
	)
	runLandlockSubprocess(t, cmd)
}

func runDescriptorIdentityHelper() {
	allowed := os.Getenv("RSHELL_LANDLOCK_FD_TEST_ALLOWED")
	moved := allowed + "-moved"
	rules, err := openRules([]string{allowed + ":ro"}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LANDLOCK_ERROR: open rules: %v\n", err)
		os.Exit(125)
	}
	defer closeOpenedRules(rules)
	if err := os.Rename(allowed, moved); err != nil {
		fmt.Fprintf(os.Stderr, "LANDLOCK_ERROR: rename allowed path: %v\n", err)
		os.Exit(125)
	}
	if err := os.Mkdir(allowed, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "LANDLOCK_ERROR: replace allowed path: %v\n", err)
		os.Exit(125)
	}
	if err := os.WriteFile(filepath.Join(allowed, "existing"), []byte("replacement"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "LANDLOCK_ERROR: populate replacement: %v\n", err)
		os.Exit(125)
	}
	if err := restrictOpenedRules(rules); err != nil {
		printRestrictError(err)
	}

	mustRead(filepath.Join(moved, "existing"))
	mustDenyRead(filepath.Join(allowed, "existing"))
}

func runLandlockSubprocess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	output, err := cmd.CombinedOutput()
	if err != nil && strings.Contains(string(output), "LANDLOCK_UNAVAILABLE:") {
		t.Skip(strings.TrimSpace(string(output)))
	}
	require.NoError(t, err, string(output))
}

func mustRestrict(allowedPaths []string, trustedPaths []TrustedPath) {
	if err := RestrictWithTrustedPaths(allowedPaths, trustedPaths); err != nil {
		printRestrictError(err)
	}
}

func printRestrictError(err error) {
	if errors.Is(err, ErrUnsupported) {
		fmt.Fprintf(os.Stderr, "LANDLOCK_UNAVAILABLE: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "LANDLOCK_ERROR: %v\n", err)
	}
	os.Exit(125)
}

func mustRead(path string) {
	if _, err := os.ReadFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}
}

func mustDenyRead(path string) {
	if _, err := os.ReadFile(path); !errors.Is(err, syscall.EACCES) {
		fmt.Fprintf(os.Stderr, "read %s: got %v, want EACCES\n", path, err)
		os.Exit(1)
	}
}

func mustWrite(path string) {
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}
}

func mustDenyWrite(path string) {
	if err := os.WriteFile(path, []byte("data"), 0o600); !errors.Is(err, syscall.EACCES) {
		fmt.Fprintf(os.Stderr, "write %s: got %v, want EACCES\n", path, err)
		os.Exit(1)
	}
}

func mustRemove(path string) {
	if err := os.Remove(path); err != nil {
		fmt.Fprintf(os.Stderr, "remove %s: %v\n", path, err)
		os.Exit(1)
	}
}

func mustDenyRemove(path string) {
	if err := os.Remove(path); !errors.Is(err, syscall.EACCES) {
		fmt.Fprintf(os.Stderr, "remove %s: got %v, want EACCES\n", path, err)
		os.Exit(1)
	}
}

func mustDenyMkdir(path string) {
	if err := os.Mkdir(path, 0o700); !errors.Is(err, syscall.EACCES) {
		fmt.Fprintf(os.Stderr, "mkdir %s: got %v, want EACCES\n", path, err)
		os.Exit(1)
	}
}

func mustDenySymlink(path string) {
	if err := os.Symlink("existing", path); !errors.Is(err, syscall.EACCES) {
		fmt.Fprintf(os.Stderr, "symlink %s: got %v, want EACCES\n", path, err)
		os.Exit(1)
	}
}

func mustDenyExecute(path string) {
	err := exec.Command(path, "-test.run=^$").Run()
	if !errors.Is(err, syscall.EACCES) {
		fmt.Fprintf(os.Stderr, "execute %s: got %v, want EACCES\n", path, err)
		os.Exit(1)
	}
}

func mustUseDevNull() {
	file, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open /dev/null: %v\n", err)
		os.Exit(1)
	}
	if _, err := file.Write([]byte("discarded")); err != nil {
		_ = file.Close()
		fmt.Fprintf(os.Stderr, "write /dev/null: %v\n", err)
		os.Exit(1)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close /dev/null: %v\n", err)
		os.Exit(1)
	}
}
