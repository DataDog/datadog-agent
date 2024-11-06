// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package paths defines commonly used paths throughout the installer
package paths

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/winregistry"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

var (
	advapi32                        = syscall.NewLazyDLL("advapi32.dll")
	procTreeResetNamedSecurityInfoW = advapi32.NewProc("TreeResetNamedSecurityInfoW")
)

var (
	// datadogInstallerData is the path to the Datadog Installer data directory, by default C:\\ProgramData\\Datadog Installer.
	datadogInstallerData string
	// PackagesPath is the path to the packages directory.
	PackagesPath string
	// ConfigsPath is the path to the Fleet-managed configuration directory
	ConfigsPath string
	// LocksPath is the path to the locks directory.
	LocksPath string
	// RootTmpDir is the temporary path where the bootstrapper will be extracted to.
	RootTmpDir string
	// DefaultUserConfigsDir is the default Agent configuration directory
	DefaultUserConfigsDir string
	// StableInstallerPath is the path to the stable installer binary.
	StableInstallerPath string
	// RunPath is the default run path
	RunPath string
)

func init() {
	datadogInstallerData, _ = winregistry.GetProgramDataDirForProduct("Datadog Installer")
	PackagesPath = filepath.Join(datadogInstallerData, "packages")
	ConfigsPath = filepath.Join(datadogInstallerData, "configs")
	LocksPath = filepath.Join(datadogInstallerData, "locks")
	RootTmpDir = filepath.Join(datadogInstallerData, "tmp")
	datadogInstallerPath := "C:\\Program Files\\Datadog\\Datadog Installer"
	StableInstallerPath = filepath.Join(datadogInstallerPath, "datadog-installer.exe")
	DefaultUserConfigsDir, _ = windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	RunPath = filepath.Join(PackagesPath, "run")
}

// CreateInstallerDataDir creates the root directory for the installer data and sets permissions
// to ensure that only Administrators have write access to the directory tree.
//
// bootstrap runs before the MSI, so it must create the directory with the correct permissions.
func CreateInstallerDataDir() error {
	targetDir := datadogInstallerData

	// Desired permissions:
	// - OWNER: Administrators
	// - GROUP: Administrators
	// - SYSTEM: Full Control (propagates to children)
	// - Administrators: Full Control (propagates to children)
	// - Everyone: 0x1200a9 List folder contents (propagates to container children only, so no access to file content)
	// - PROTECTED: does not inherit permissions from parent
	sddl := "O:BAG:BAD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;CI;0x1200a9;;;WD)"

	// The following privileges are required to modify the security descriptor,
	// and are granted to Administrators by default:
	//  - SeTakeOwnershipPrivilege - Required to set the owner
	privilegesRequired := []string{"SeTakeOwnershipPrivilege"}
	return winio.RunWithPrivileges(privilegesRequired, func() error {
		return secureCreateDirectory(targetDir, sddl)
	})
}

func secureCreateDirectory(path string, sddl string) error {
	// Try to create the directory with the desired permissions.
	// We avoid TOCTOU issues because CreateDirectory fails if the directory already exists.
	// This is of concern because Windows by default grants Users write access to ProgramData.
	err := createDirectoryWithSDDL(path, sddl)
	if err != nil {
		if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			// return other errors, ERROR_ALREADY_EXISTS is handled below
			return err
		}
	}
	if err == nil {
		// directory securely created, we're done
		return nil
	}

	// creation failed because the directory already exists.
	// Our options here are to:
	// (a) Fail the install because the directory was created by an unknown party
	// (b) Attempt to reset the permissions to the expected state
	// We choose option (b) because it allows us to modify the permissions in the future.
	// We check the owner to ensure it is Administrators or SYSTEM before changing the permissions,
	// as the owner cannot be set to Administrators by a non-privileged user.
	err = isDirSecure(path)
	if err != nil {
		// The directory owner is not Administrators or SYSTEM, so may have been created
		// by an unknown party. Adjusting the permissions may not be safe, as it won't affect
		// already open handles, so we fail the install.
		return err
	}

	// The owner is Administrators or SYSTEM, so we can be resonably sure the directory and its
	// original permissions were created by an Administrator. If the Administrator created
	// the directory insecurely, we'll reset the permissions here, but we can't account
	// for damage that might have already been done.
	err = treeResetNamedSecurityInfoWithSDDL(path, sddl)
	if err != nil {
		return err
	}

	return nil
}

// IsInstallerDataDirSecure return nil if the Datadog Installer data directory is owned by Administrators or SYSTEM,
// otherwise an error is returned.
//
// CreateInstallerDataDir sets the owner to Administrators and is called during bootstrap.
// Unprivileged users (users without SeTakeOwnershipPrivilege/SeRestorePrivilege) cannot set the owner to Administrators.
func IsInstallerDataDirSecure() error {
	targetDir := datadogInstallerData
	return isDirSecure(targetDir)
}

func isDirSecure(targetDir string) error {
	allowedWellKnownSids := []windows.WELL_KNOWN_SID_TYPE{
		windows.WinBuiltinAdministratorsSid,
		windows.WinLocalSystemSid,
	}

	// get security info
	sd, err := windows.GetNamedSecurityInfo(targetDir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("failed to get security info: %w", err)
	}
	// ensure owner is admin or system
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("failed to get owner: %w", err)
	}
	if owner == nil {
		return fmt.Errorf("owner is nil")
	}
	var allowedSids []*windows.SID
	for _, id := range allowedWellKnownSids {
		sid, err := windows.CreateWellKnownSid(id)
		if err != nil {
			return fmt.Errorf("failed to create well known sid %v: %w", id, err)
		}
		allowedSids = append(allowedSids, sid)
	}
	ownerInAllowedList := slices.ContainsFunc(allowedSids, func(sid *windows.SID) bool {
		return windows.EqualSid(owner, sid)
	})
	if !ownerInAllowedList {
		return fmt.Errorf("installer data directory has unexpected owner: %v", owner.String())
	}
	return nil
}

// createDirectoryWithSDDL creates a directory with the specified SDDL string, returns
// an error if the directory already exists.
func createDirectoryWithSDDL(path string, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("failed to create security descriptor from sddl %s: %w", sddl, err)
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
		InheritHandle:      0,
	}

	// create directory with security descriptor
	err = windows.CreateDirectory(windows.StringToUTF16Ptr(path), sa)
	if err != nil {
		// CreateDirectory does not apply the security descriptor if the directory already exists
		// so treat it as an error if the directory already exists.
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

func treeResetNamedSecurityInfoWithSDDL(root string, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	return treeResetNamedSecurityInfoFromSecurityDescriptor(root, sd)
}

func treeResetNamedSecurityInfoFromSecurityDescriptor(root string, sd *windows.SECURITY_DESCRIPTOR) error {
	var flags windows.SECURITY_INFORMATION
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	flags |= securityInformationFromControlFlags(control)

	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner != nil {
		flags |= windows.OWNER_SECURITY_INFORMATION
	}
	group, _, err := sd.Group()
	if err != nil {
		return err
	}
	if group != nil {
		flags |= windows.GROUP_SECURITY_INFORMATION
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		if err != windows.ERROR_OBJECT_NOT_FOUND {
			return err
		}
	} else {
		flags |= windows.DACL_SECURITY_INFORMATION
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		if err != windows.ERROR_OBJECT_NOT_FOUND {
			return err
		}
	} else {
		flags |= windows.SACL_SECURITY_INFORMATION
	}
	err = TreeResetNamedSecurityInfo(
		root,
		windows.SE_FILE_OBJECT,
		flags,
		owner,
		group,
		dacl,
		sacl,
		// Set to false to remove explicit ACEs from the subtree
		false)
	if err != nil {
		return err
	}
	return nil
}

func securityInformationFromControlFlags(control windows.SECURITY_DESCRIPTOR_CONTROL) windows.SECURITY_INFORMATION {
	var flags windows.SECURITY_INFORMATION
	if control&windows.SE_DACL_PROTECTED == 0 {
		flags |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	} else {
		flags |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	if control&windows.SE_SACL_PROTECTED == 0 {
		flags |= windows.UNPROTECTED_SACL_SECURITY_INFORMATION
	} else {
		flags |= windows.PROTECTED_SACL_SECURITY_INFORMATION
	}
	return flags
}

// TreeResetNamedSecurityInfo wraps the TreeResetNamedSecurityInfoW Windows API function
//
// https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-treeresetnamedsecurityinfow
func TreeResetNamedSecurityInfo(
	objectName string,
	objectType windows.SE_OBJECT_TYPE,
	securityInfo windows.SECURITY_INFORMATION,
	owner *windows.SID,
	group *windows.SID,
	dacl *windows.ACL,
	sacl *windows.ACL,
	keepExplicitDacl bool) error {

	utf16ObjectName, err := windows.UTF16PtrFromString(objectName)
	if err != nil {
		return err
	}

	keepExplicitDaclInt := uintptr(0)
	if keepExplicitDacl {
		keepExplicitDaclInt = 1
	}

	r0, _, _ := procTreeResetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(utf16ObjectName)),
		uintptr(objectType),
		uintptr(securityInfo),
		uintptr(unsafe.Pointer(owner)),
		uintptr(unsafe.Pointer(group)),
		uintptr(unsafe.Pointer(dacl)),
		uintptr(unsafe.Pointer(sacl)),
		keepExplicitDaclInt,
		// don't use a progress callback
		0, 0, 0)
	if r0 != 0 {
		return syscall.Errno(r0)
	}
	return nil
}
