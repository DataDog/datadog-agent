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
	"testing"
)

func TestSecureCreateDirectory(t *testing.T) {
	t.Run("new directory", func(t *testing.T) {
		root := t.TempDir()
		subdir := filepath.Join(root, "A")
		sddl := "D:PAI(A;OICI;FA;;;AU)"
		err := secureCreateDirectory(subdir, sddl)
		require.NoError(t, err)
		sd, err := getSecurityDescriptor(subdir)
		require.NoError(t, err)
		assertDACLProtected(t, sd)
	})

	t.Run("directory exists", func(t *testing.T) {
		t.Run("unknown owner", func(t *testing.T) {
			root := t.TempDir()
			subdir := filepath.Join(root, "A")
			err := os.Mkdir(subdir, 0)
			require.NoError(t, err)
			sddl := "O:BAG:BAD:PAI(A;OICI;FA;;;AU)"
			err = secureCreateDirectory(subdir, sddl)
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
			err = secureCreateDirectory(subdir, sddl)
			require.NoError(t, err)
			sd, err := getSecurityDescriptor(subdir)
			require.NoError(t, err)
			assertDACLProtected(t, sd)
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

func assertDACLProtected(t *testing.T, sd *windows.SECURITY_DESCRIPTOR) {
	t.Helper()
	control, _, err := sd.Control()
	require.NoError(t, err)
	assert.NotZero(t, control&windows.SE_DACL_PROTECTED)
}
