// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package landlock

import (
	"os"
	"path/filepath"
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
