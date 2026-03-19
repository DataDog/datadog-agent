// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package paths

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/fs"
	"syscall"
	"testing"
)

func TestSecureCreateDirectory(t *testing.T) {
	t.Run("new directory", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "A")
		sddl := "D:PAI(A;OICI;FA;;;AU)"
		err := SecureCreateDirectory(subdir, sddl)
		require.NoError(t, err)
		sd, err := getSecurityDescriptor(subdir)
		require.NoError(t, err)
		assertDACLProtected(t, sd)
		assertDACLAutoInherit(t, sd)
		sd, err = windows.GetNamedSecurityInfo(subdir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		require.NoError(t, err)
		assert.Equal(t, sddl, sd.String())
	})

	t.Run("directory exists", func(t *testing.T) {
		t.Run("unknown owner", func(t *testing.T) {
			root := t.TempDir()
			subdir := filepath.Join(root, "A")
			err := os.Mkdir(subdir, 0)
			require.NoError(t, err)
			sddl := "O:BAG:BAD:PAI(A;OICI;FA;;;AU)"
			err = SecureCreateDirectory(subdir, sddl)
			require.Error(t, err)
			assert.ErrorContains(t, err, "installer data directory has unexpected owner")
		})
		t.Run("known owner", func(t *testing.T) {
			// required to set owner to another user
			privilegesRequired := []string{"SeRestorePrivilege"}
			skipIfDontHavePrivileges(t, privilegesRequired)
			root := t.TempDir()
			subdir := filepath.Join(root, "A")
			sddl := "O:SYG:SYD:PAI(A;OICI;FA;;;AU)"
			err := winio.RunWithPrivileges(privilegesRequired, func() error {
				return createDirectoryWithSDDL(subdir, sddl)
			})
			require.NoError(t, err)
			sddl = "O:BAG:BAD:PAI(A;OICI;FA;;;AU)"
			err = SecureCreateDirectory(subdir, sddl)
			require.NoError(t, err)
			sd, err := getSecurityDescriptor(subdir)
			require.NoError(t, err)
			assertDACLProtected(t, sd)
			assert.Equal(t, sddl, sd.String())
		})
	})
}

func skipIfDontHavePrivileges(t *testing.T, privilegesRequired []string) {
	user, err := user.Current()
	require.NoError(t, err)
	if os.Getenv("CI") != "" || os.Getenv("CI_JOB_ID") != "" || strings.Contains(user.Name, "ContainerAdministrator") {
		// never skip in CI, we should always have the required privileges and we
		// want the test to run
		return
	}
	hasPrivs := false
	err = winio.RunWithPrivileges(privilegesRequired, func() error {
		hasPrivs = true
		return nil
	})
	if err != nil || !hasPrivs {
		t.Skipf("test requires %v", strings.Join(privilegesRequired, ","))
	}
}

func getSecurityDescriptor(path string) (*windows.SECURITY_DESCRIPTOR, error) {
	return windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
}

// assertDACLProtected asserts that the DACL is protected, which ensure it does not inherit ACEs from parents
func assertDACLProtected(t *testing.T, sd *windows.SECURITY_DESCRIPTOR) {
	t.Helper()
	control, _, err := sd.Control()
	require.NoError(t, err)
	assert.NotZero(t, control&windows.SE_DACL_PROTECTED)
}

// assertDACLAutoInherit asserts that the DACL is auto inherited, which ensures it propagates ACEs to children
func assertDACLAutoInherit(t *testing.T, sd *windows.SECURITY_DESCRIPTOR) {
	t.Helper()
	control, _, err := sd.Control()
	require.NoError(t, err)
	assert.NotZero(t, control&windows.SE_DACL_AUTO_INHERITED)
}

func getDACLSDDL(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	return sd.String()
}

func assertDACLNotProtected(t *testing.T, sd *windows.SECURITY_DESCRIPTOR) {
	t.Helper()
	control, _, err := sd.Control()
	require.NoError(t, err)
	assert.Zero(t, control&windows.SE_DACL_PROTECTED, "DACL should not be protected")
}

const fileAllAccess = 0x1F01FF

func TestAddExplicitAccessToFile(t *testing.T) {
	t.Run("adds ACE and preserves inheritance", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0))

		sid, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
		require.NoError(t, err)

		require.NotContains(t, getDACLSDDL(t, filePath), "BG", "precondition: Guests should not have access")

		err = AddExplicitAccessToFile(filePath, sid, fileAllAccess)
		require.NoError(t, err)

		assert.Contains(t, getDACLSDDL(t, filePath), "BG")
		sd, err := getSecurityDescriptor(filePath)
		require.NoError(t, err)
		assertDACLNotProtected(t, sd)
	})

	t.Run("file does not exist", func(t *testing.T) {
		sid, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
		require.NoError(t, err)
		err = AddExplicitAccessToFile(filepath.Join(t.TempDir(), "nonexistent.txt"), sid, fileAllAccess)
		require.Error(t, err)
	})
}

func TestRevokeExplicitAccessFromFile(t *testing.T) {
	t.Run("removes ACE and preserves inheritance", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0))

		sid, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
		require.NoError(t, err)

		require.NoError(t, AddExplicitAccessToFile(filePath, sid, fileAllAccess))
		require.Contains(t, getDACLSDDL(t, filePath), "BG")

		err = RevokeExplicitAccessFromFile(filePath, sid)
		require.NoError(t, err)

		assert.NotContains(t, getDACLSDDL(t, filePath), "BG")
		sd, err := getSecurityDescriptor(filePath)
		require.NoError(t, err)
		assertDACLNotProtected(t, sd)
	})

	t.Run("preserves other explicit ACEs", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0))

		guestsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
		require.NoError(t, err)
		boSID, err := windows.CreateWellKnownSid(windows.WinBuiltinBackupOperatorsSid)
		require.NoError(t, err)

		require.NoError(t, AddExplicitAccessToFile(filePath, guestsSID, fileAllAccess))
		require.NoError(t, AddExplicitAccessToFile(filePath, boSID, fileAllAccess))

		err = RevokeExplicitAccessFromFile(filePath, guestsSID)
		require.NoError(t, err)

		sddl := getDACLSDDL(t, filePath)
		assert.NotContains(t, sddl, "BG")
		assert.Contains(t, sddl, "BO")
	})

	t.Run("noop when SID has no ACE", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "test.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("test"), 0))

		sid, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
		require.NoError(t, err)

		err = RevokeExplicitAccessFromFile(filePath, sid)
		require.NoError(t, err)
		assert.NotContains(t, getDACLSDDL(t, filePath), "BG")
	})

	t.Run("file does not exist", func(t *testing.T) {
		sid, err := windows.CreateWellKnownSid(windows.WinBuiltinGuestsSid)
		require.NoError(t, err)
		err = RevokeExplicitAccessFromFile(filepath.Join(t.TempDir(), "nonexistent.txt"), sid)
		require.Error(t, err)
	})
}

func TestCreateDirIfNotExists(t *testing.T) {
	t.Run("directory does not exist", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "newdir")
		err := createDirIfNotExists(subdir)
		require.NoError(t, err)
		info, err := os.Stat(subdir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("directory already exists", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "existingdir")
		err := os.Mkdir(subdir, 0)
		require.NoError(t, err)
		err = createDirIfNotExists(subdir)
		require.NoError(t, err)
		info, err := os.Stat(subdir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("path exists but is not a directory", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "notadir")
		err := os.WriteFile(file, []byte("test"), 0)
		require.NoError(t, err)
		err = createDirIfNotExists(file)
		require.Error(t, err)
		var pathErr *fs.PathError
		require.ErrorAs(t, err, &pathErr)
		assert.Equal(t, syscall.ENOTDIR, pathErr.Err)
	})

	t.Run("parent directory does not exist", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "nonexistent", "subdir")
		err := createDirIfNotExists(subdir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create directory")
	})
}
