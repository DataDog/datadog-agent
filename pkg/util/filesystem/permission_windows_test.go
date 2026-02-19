// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestRemoveAccessToOtherUsers(t *testing.T) {

	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()

	t.Log(root)

	testFile := filepath.Join(root, "file")
	testDir := filepath.Join(root, "dir")

	err = os.WriteFile(testFile, []byte("test"), 0777)
	require.NoError(t, err)
	err = os.Mkdir(testDir, 0777)
	require.NoError(t, err)

	err = p.RemoveAccessToOtherUsers(testFile)
	require.NoError(t, err)

	// Assert the permissions for the file
	assertPermissions(t, testFile, p)

	err = p.RemoveAccessToOtherUsers(testDir)
	require.NoError(t, err)

	// Assert the permissions for the directory
	assertPermissions(t, testDir, p)
}

func assertPermissions(t *testing.T, path string, p *Permission) {
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	require.NoError(t, err)

	dacl, _, err := sd.DACL()
	require.NoError(t, err)

	aceCount := int(dacl.AceCount)
	for i := 0; i < aceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		err := windows.GetAce(dacl, uint32(i), &ace)
		require.NoError(t, err)

		var sid *windows.SID = (*windows.SID)(unsafe.Pointer(&ace.SidStart))

		if !sid.Equals(p.ddUserSid) && !sid.Equals(p.administratorSid) && !sid.Equals(p.systemSid) {
			t.Errorf("Unexpected SID with access: %v", sid)
		}
	}
}

// setFileOwner sets the owner of path to the given SID. Requires admin or WRITE_OWNER.
// Returns an error if the caller cannot set the owner.
func setFileOwner(path string, ownerSid *windows.SID) error {
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
		ownerSid,
		nil,
		nil,
		nil,
	)
}

func TestCheckOwner_CurrentUser(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	testFile := filepath.Join(root, "file")

	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// File created by current user, should pass ownership check
	err = p.checkOwner(testFile)
	assert.NoError(t, err)
}

func TestCheckOwner_Administrator(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	testFile := filepath.Join(root, "file")

	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Only a process with sufficient privileges can set owner to Administrator
	if err := setFileOwner(testFile, p.administratorSid); err != nil {
		t.Skip("Cannot set file owner to Administrator")
	}

	err = p.checkOwner(testFile)
	assert.NoError(t, err)
}

func TestCheckOwner_System(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()
	testFile := filepath.Join(root, "file")

	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Only a process with sufficient privileges can set owner to SYSTEM
	if err := setFileOwner(testFile, p.systemSid); err != nil {
		t.Skip("Cannot set file owner to SYSTEM")
	}

	err = p.checkOwner(testFile)
	assert.NoError(t, err)
}

func TestCheckOwner_DDAgent(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	currentSid, err := winutil.GetSidFromUser()
	require.NoError(t, err)

	// If the current user is the dd user, the file created by the current user (dd user) should pass ownership check
	if windows.EqualSid(currentSid, p.ddUserSid) {
		root := t.TempDir()
		testFile := filepath.Join(root, "file")
		err = os.WriteFile(testFile, []byte("test"), 0644)
		require.NoError(t, err)
		err = p.checkOwner(testFile)
		assert.NoError(t, err)
		return
	}

	// Otherwise we need to set owner to dd user
	root := t.TempDir()
	testFile := filepath.Join(root, "file")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	if err := setFileOwner(testFile, p.ddUserSid); err != nil {
		t.Skip("Cannot set file owner to dd user")
	}

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

func TestIsAllowedOwner_Administrator(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)
	assert.True(t, p.isAllowedOwner(p.administratorSid))
}

func TestIsAllowedOwner_System(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)
	assert.True(t, p.isAllowedOwner(p.systemSid))
}

func TestIsAllowedOwner_DDAgent(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)
	assert.True(t, p.isAllowedOwner(p.ddUserSid))
}

func TestIsAllowedOwner_UnknownUser(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	// user other than Administrator, SYSTEM, dd user
	usersSid, err := windows.StringToSid("S-1-5-32-545")
	require.NoError(t, err)
	assert.False(t, p.isAllowedOwner(usersSid))
}
