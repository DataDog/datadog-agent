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

// TestSetRepositoryPermissionsBeforeRenameBlocksMove is a regression test for
// the ordering bug fixed in PR #50620. SetRepositoryPermissions applies a
// protected DACL via TreeResetNamedSecurityInfoW that severs inheritance and,
// in UAC-filtered contexts, removes the process's DELETE right — causing the
// subsequent MoveFileEx to fail with ERROR_ACCESS_DENIED.
//
// Part 1 demonstrates the failure mode using a fully-restrictive DACL
// (Everyone read+execute only, no DELETE for anyone) so the assertion is
// reliable regardless of the CI process privilege level.
// Part 2 demonstrates the fix: rename first, then apply permissions.
func TestSetRepositoryPermissionsBeforeRenameBlocksMove(t *testing.T) {
	root := t.TempDir()

	// Part 1: permissions BEFORE rename — the old, broken order.
	srcBug := filepath.Join(root, "src-bug")
	dstBug := filepath.Join(root, "dst-bug")
	require.NoError(t, os.Mkdir(srcBug, 0o755))

	// Apply a fully-restrictive protected DACL (Everyone R+X, no DELETE for
	// anyone including SYSTEM) to simulate what SetRepositoryPermissions does in
	// non-elevated contexts where the Administrators SID is filtered.
	require.NoError(t, treeResetNamedSecurityInfoWithSDDL(srcBug, "D:PAI(A;OICI;0x1200A9;;;WD)"))
	// Register cleanup before assertions so it runs (LIFO) before t.TempDir's
	// cleanup and can restore the DACL for os.RemoveAll.
	t.Cleanup(func() {
		_ = treeResetNamedSecurityInfoWithSDDL(srcBug, "D:PAI(A;OICI;FA;;;WD)")
	})

	// Use os.Rename directly (not paths.Rename) to avoid the 60-second retry loop.
	err := os.Rename(srcBug, dstBug)
	require.Error(t, err, "rename after locking DACL should fail with ERROR_ACCESS_DENIED")
	var linkErr *os.LinkError
	require.ErrorAs(t, err, &linkErr)
	assert.ErrorIs(t, linkErr.Err, windows.ERROR_ACCESS_DENIED)

	// Part 2: rename BEFORE permissions — the fix.
	srcFix := filepath.Join(root, "src-fix")
	dstFix := filepath.Join(root, "dst-fix")
	require.NoError(t, os.Mkdir(srcFix, 0o755))

	require.NoError(t, Rename(t.Context(), srcFix, dstFix))
	require.NoError(t, SetRepositoryPermissions(dstFix))
	assert.DirExists(t, dstFix)
	assert.NoDirExists(t, srcFix)

	sd, err := getSecurityDescriptor(dstFix)
	require.NoError(t, err)
	assertDACLProtected(t, sd)
	assertDACLAutoInherit(t, sd)
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
