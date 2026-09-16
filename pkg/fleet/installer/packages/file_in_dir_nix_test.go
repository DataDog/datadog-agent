// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
)

func TestEnsurePermissionsInDirRefusesSymlink(t *testing.T) {
	cfgDir, victim := symlinkVictimFixture(t, "victim content\n")
	require.NoError(t, os.Symlink(victim, filepath.Join(cfgDir, "otel-config.yaml")))

	ctx := HookContext{Context: t.Context()}
	err := ensurePermissionsInDir(ctx, cfgDir, file.Permissions{{Path: "otel-config.yaml", Mode: 0o640}})
	require.ErrorContains(t, err, "it is a symlink")

	info, err := os.Stat(victim)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"the file the symlink points at must not be re-permissioned")
}

func TestEnsurePermissionsInDirAppliesMode(t *testing.T) {
	cfgDir := t.TempDir()
	configPath := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("config\n"), 0o600))

	ctx := HookContext{Context: t.Context()}
	require.NoError(t, ensurePermissionsInDir(ctx, cfgDir, file.Permissions{{Path: "otel-config.yaml", Mode: 0o640}}))

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestEnsurePermissionsInDirSkipsMissing(t *testing.T) {
	ctx := HookContext{Context: t.Context()}
	require.NoError(t, ensurePermissionsInDir(ctx, t.TempDir(), file.Permissions{{Path: "otel-config.yaml", Mode: 0o640}}))
	require.NoError(t, ensurePermissionsInDir(ctx, filepath.Join(t.TempDir(), "absent"), file.Permissions{{Path: "otel-config.yaml", Mode: 0o640}}))
}

func TestEnsurePermissionsInDirRejectsRecursive(t *testing.T) {
	ctx := HookContext{Context: t.Context()}
	err := ensurePermissionsInDir(ctx, t.TempDir(), file.Permissions{{Path: ".", Mode: 0o640, Recursive: true}})
	require.ErrorContains(t, err, "not supported")
}

// The ownership half of ensurePermissionsInDir needs root, so these are gated. Without them
// root.Chown is never executed by any test, even though it is the operation the finding behind
// this code is about.

func TestEnsurePermissionsInDirAppliesOwnership(t *testing.T) {
	owner, group, uid, gid := unprivilegedOwner(t)

	cfgDir := t.TempDir()
	configPath := filepath.Join(cfgDir, "otel-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("config\n"), 0o600))

	ctx := HookContext{Context: t.Context()}
	require.NoError(t, ensurePermissionsInDir(ctx, cfgDir,
		file.Permissions{{Path: "otel-config.yaml", Owner: owner, Group: group, Mode: 0o640}}))

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "unexpected FileInfo.Sys type")
	assert.Equal(t, uint32(uid), stat.Uid)
	assert.Equal(t, uint32(gid), stat.Gid)
}

func TestEnsurePermissionsInDirDoesNotChownSymlinkTarget(t *testing.T) {
	owner, group, _, _ := unprivilegedOwner(t)

	cfgDir, victim := symlinkVictimFixture(t, "victim content\n")
	require.NoError(t, os.Chown(victim, 0, 0))
	require.NoError(t, os.Symlink(victim, filepath.Join(cfgDir, "otel-config.yaml")))

	ctx := HookContext{Context: t.Context()}
	err := ensurePermissionsInDir(ctx, cfgDir,
		file.Permissions{{Path: "otel-config.yaml", Owner: owner, Group: group, Mode: 0o640}})
	require.ErrorContains(t, err, "it is a symlink")

	info, err := os.Stat(victim)
	require.NoError(t, err)
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok, "unexpected FileInfo.Sys type")
	assert.Equal(t, uint32(0), stat.Uid, "the root-owned target must keep its owner")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "and its mode")
}

// unprivilegedOwner returns a user and group that exist on the host and are not root, skipping
// the test when ownership cannot be changed.
func unprivilegedOwner(t *testing.T) (string, string, int, int) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("test requires root to change file ownership")
	}
	var owner, group string
	var uid, gid int
	for _, name := range []string{"nobody", "daemon"} {
		if u, err := user.Lookup(name); err == nil {
			if parsed, err := strconv.Atoi(u.Uid); err == nil && parsed != 0 {
				owner, uid = name, parsed
				break
			}
		}
	}
	for _, name := range []string{"nogroup", "nobody", "daemon"} {
		if g, err := user.LookupGroup(name); err == nil {
			if parsed, err := strconv.Atoi(g.Gid); err == nil && parsed != 0 {
				group, gid = name, parsed
				break
			}
		}
	}
	if owner == "" || group == "" {
		t.Skip("no unprivileged user and group available on this host")
	}
	return owner, group, uid, gid
}
