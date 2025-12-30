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

func TestGetOwner_FileExists(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "file")
	require.NoError(t, err)

	ownerName, err := p.GetOwner(tempFile.Name())
	require.NoError(t, err)

	// owner of the temp file is the current user executing the test
	user, err := user.Current()
	require.NoError(t, err)

	assert.Equal(t, user.Username, ownerName)
}

func TestGetOwner_NoFile(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	ownerName, err := p.GetOwner("path/to/nothing")
	assert.Empty(t, ownerName)
	assert.EqualError(t, err, "could not stat path/to/nothing: no such file or directory")

}

func TestGetOtherUsersWriteAccess_CanWrite(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "")
	require.NoError(t, err)

	// give other users the write access to the temp file
	err = os.Chmod(tempFile.Name(), 0757)
	require.NoError(t, err)

	otherWriteAccess, err := p.GetOtherUsersWriteAccess(tempFile.Name())
	require.NoError(t, err)

	assert.True(t, otherWriteAccess)
}

func TestGetOtherUsersWriteAccess_CannotWrite(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	tempDir := t.TempDir()
	tempFile, err := os.CreateTemp(tempDir, "")
	require.NoError(t, err)

	// remove other users write access to the temp file
	err = os.Chmod(tempFile.Name(), 0755)
	require.NoError(t, err)

	otherWriteAccess, err := p.GetOtherUsersWriteAccess(tempFile.Name())
	require.NoError(t, err)

	assert.False(t, otherWriteAccess)
}
