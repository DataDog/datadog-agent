// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveAccessToOtherUsers(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()

	testFile := filepath.Join(root, "file")
	testDir := filepath.Join(root, "dir")

	err = os.WriteFile(testFile, []byte("test"), 0777)
	require.NoError(t, err)
	err = os.Mkdir(testDir, 0777)
	require.NoError(t, err)

	err = p.RemoveAccessToOtherUsers(testFile)
	require.NoError(t, err)
	stat, err := os.Stat(testFile)
	require.NoError(t, err)
	assert.Equal(t, int(stat.Mode().Perm()), 0700)

	err = p.RemoveAccessToOtherUsers(testDir)
	require.NoError(t, err)
	stat, err = os.Stat(testDir)
	require.NoError(t, err)
	assert.Equal(t, int(stat.Mode().Perm()), 0700)
}

func TestGetDatadogUserUID(t *testing.T) {
	uid, err := getDatadogUserUID()
	require.NoError(t, err)

	ddAgentUser, lookupErr := user.Lookup(agentUsername())
	if lookupErr != nil {
		// agent user not found, should fall back to current user
		assert.Equal(t, uint32(os.Getuid()), uid)
	} else {
		assert.Equal(t, ddAgentUser.Uid, strconv.FormatUint(uint64(uid), 10))
	}
}

func TestCheckOwner_Root(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	testFile := filepath.Join(root, "file")

	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Only root can chown to root (uid 0)
	if err := os.Chown(testFile, 0, 0); err != nil {
		t.Skip("Cannot chown to root")
	}

	// File owned by root should pass ownership check
	err = p.checkOwner(testFile)
	assert.NoError(t, err)
}

func TestCheckOwner_DDAgent(t *testing.T) {
	ddAgentUser, err := user.Lookup(agentUsername())
	if err != nil {
		t.Skip("agent user not found on this system")
	}
	ddAgentUID, err := strconv.ParseUint(ddAgentUser.Uid, 10, 32)
	require.NoError(t, err)
	gid, err := strconv.Atoi(ddAgentUser.Gid)
	require.NoError(t, err)

	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	testFile := filepath.Join(root, "file")

	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Only root can chown to another user
	if err := os.Chown(testFile, int(ddAgentUID), gid); err != nil {
		t.Skip("Cannot chown to dd-agent (run as root to test)")
	}

	// File owned by dd-agent should pass ownership check
	err = p.checkOwner(testFile)
	assert.NoError(t, err)
}

func TestCheckOwner_NonExistentFile(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	nonExistentFile := filepath.Join(t.TempDir(), "non_existent")
	err = p.checkOwner(nonExistentFile)
	assert.Error(t, err)
}

func TestIsAllowedOwner_Root(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)
	assert.True(t, p.isRootOrAgentUID(0))
}

func TestIsAllowedOwner_DDAgent(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	ddAgentUID, err := getDatadogUserUID()
	require.NoError(t, err)

	assert.True(t, p.isRootOrAgentUID(ddAgentUID))
}

func TestIsAllowedOwner_UnknownUser(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)
	unknownUID := uint32(99999)
	assert.False(t, p.isRootOrAgentUID(unknownUID))
}

func TestRestrictGroupAccessNoFollow(t *testing.T) {
	ddAgentUser, err := user.Lookup(agentUsername())
	if err != nil {
		t.Skip("agent user not found on this system")
	}
	ddAgentGID, err := strconv.Atoi(ddAgentUser.Gid)
	require.NoError(t, err)

	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	testFile := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	// Only root can chown to another group.
	if err := os.Lchown(testFile, -1, ddAgentGID); err != nil {
		t.Skip("Cannot chown to the dd-agent group (run as root to test)")
	}
	require.NoError(t, os.Lchown(testFile, -1, os.Getgid()))

	require.NoError(t, p.RestrictGroupAccessNoFollow(testFile))

	stat, err := os.Stat(testFile)
	require.NoError(t, err)
	sys := stat.Sys().(*syscall.Stat_t)
	assert.Equal(t, ddAgentGID, int(sys.Gid))
	assert.Equal(t, os.Getuid(), int(sys.Uid), "the user owner must be left alone")
}

// The point of the no-follow variant: a symlink planted at the path must not get the
// privileged caller to change the group of whatever it points at.
func TestRestrictGroupAccessNoFollowDoesNotFollowSymlinks(t *testing.T) {
	ddAgentUser, err := user.Lookup(agentUsername())
	if err != nil {
		t.Skip("agent user not found on this system")
	}
	ddAgentGID, err := strconv.Atoi(ddAgentUser.Gid)
	require.NoError(t, err)

	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	target := filepath.Join(root, "target")
	require.NoError(t, os.WriteFile(target, []byte("test"), 0644))

	// Only root can chown to another group.
	if err := os.Lchown(target, -1, ddAgentGID); err != nil {
		t.Skip("Cannot chown to the dd-agent group (run as root to test)")
	}
	require.NoError(t, os.Lchown(target, -1, os.Getgid()))

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, p.RestrictGroupAccessNoFollow(link))

	stat, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.Getgid(), int(stat.Sys().(*syscall.Stat_t).Gid), "the symlink target's group must be untouched")
}
