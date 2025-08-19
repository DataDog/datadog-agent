// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package filesystem

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
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
