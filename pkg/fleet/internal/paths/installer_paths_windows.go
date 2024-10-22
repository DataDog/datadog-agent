// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package paths defines commonly used paths throughout the installer
package paths

import (
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/winregistry"
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
func CreateInstallerDataDir() error {
	// Since bootstrap can run and use this path before the Installer MSI creates it
	// we need to make sure the path is created with the correct permissions.
	// This is of concern because Windows by default grants Users write access to ProgramData.
	// Permissions:
	// - OWNER: Administrators
	// - GROUP: Administrators
	// - SYSTEM: Full Control (propagates to children)
	// - Administrators: Full Control (propagates to children)
	// - PROTECTED: does not inherit permissions from parent
	sddl := "O:BAGBA:D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	err := createDirectoryAndResetTreeWithSDDL(datadogInstallerData, sddl)
	if err != nil {
		return err
	}

	return nil
}

func createDirectoryAndResetTreeWithSDDL(path string, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
		InheritHandle:      0,
	}

	// create directory with security descriptor
	err = windows.CreateDirectory(windows.StringToUTF16Ptr(path), sa)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return err
	}
	// CreateDirectory does NOT apply the security descriptor if the directory already exists
	// so we need to set it explicitly.
	// Additionally, if the directory/tree already exists, we need to reset the children, too.
	err = treeResetNamedSecurityInfoFromSecurityDescriptor(path, sd)
	if err != nil {
		return err
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
