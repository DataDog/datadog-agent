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
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	advapi32                        = syscall.NewLazyDLL("advapi32.dll")
	procTreeResetNamedSecurityInfoW = advapi32.NewProc("TreeResetNamedSecurityInfoW")
)

var (
	// DatadogDataDir is the path to the Datadog data directory, by default C:\\ProgramData\\Datadog.
	// This is configurable via the DD_APPLICATIONDATADIRECTORY environment variable.
	DatadogDataDir string
	// DatadogProgramFilesDir Datadog Program Files directory
	// This is configurable via the DD_PROJECTLOCATION environment variable.
	DatadogProgramFilesDir string
	// DatadogInstallerData is the path to the Datadog Installer data directory, by default C:\\ProgramData\\Datadog\\Installer.
	DatadogInstallerData string
	// PackagesPath is the path to the packages directory.
	PackagesPath string
	// ConfigsPath is the path to the Fleet-managed configuration directory
	ConfigsPath string
	// RootTmpDir is the temporary path where the bootstrapper will be extracted to.
	RootTmpDir string
	// DefaultUserConfigsDir is the default Agent configuration directory
	DefaultUserConfigsDir string
	// StableInstallerPath is the path to the stable installer binary.
	StableInstallerPath string
	// RunPath is the default run path
	RunPath string
)

// securityInfo holds the security information extracted from a security
// descriptor for use in Windows API calls such as SetNamedSecurityInfo.
type securityInfo struct {
	Flags windows.SECURITY_INFORMATION
	Owner *windows.SID
	Group *windows.SID
	DACL  *windows.ACL
	SACL  *windows.ACL
}

func init() {
	// Fetch environment variables, the paths are configurable.
	// setup and experiment subcommands will respect the paths configured in the environment.
	// This is important for experiments, as running the MSI may remove the registry keys.
	// The daemon should expect to only read the paths from the registry, but there's no way to
	// differentiate the two runtime environments here.
	env := env.FromEnv()

	// OS paths
	DefaultUserConfigsDir, _ = windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)

	// Data directory
	if env.MsiParams.ApplicationDataDirectory != "" {
		DatadogDataDir = env.MsiParams.ApplicationDataDirectory
	} else {
		DatadogDataDir, _ = getProgramDataDirForProduct("Datadog Agent")
	}
	DatadogInstallerData = filepath.Join(DatadogDataDir, "Installer")
	PackagesPath = filepath.Join(DatadogInstallerData, "packages")
	ConfigsPath = filepath.Join(DatadogInstallerData, "managed")
	RootTmpDir = filepath.Join(DatadogInstallerData, "tmp")
	RunPath = filepath.Join(PackagesPath, "run")

	// Install directory
	if env.MsiParams.ProjectLocation != "" {
		DatadogProgramFilesDir = env.MsiParams.ProjectLocation
	} else {
		DatadogProgramFilesDir, _ = getProgramFilesDirForProduct("Datadog Agent")
	}
	StableInstallerPath = filepath.Join(DatadogProgramFilesDir, "bin", "datadog-installer.exe")
}

// createDirIfNotExists creates a directory if it doesn't exist.
// Returns an error if the path exists but is not a directory, or if creation fails.
//
// Function behaves similarly to os.MkdirAll, but does not create parent directories.
func createDirIfNotExists(path string) error {
	// Check if directory exists first
	info, err := os.Stat(path)
	if err == nil {
		// Path exists, verify it's a directory
		if !info.IsDir() {
			return &fs.PathError{
				Op:   "mkdir",
				Path: path,
				Err:  syscall.ENOTDIR,
			}
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		// Some other error occurred while checking
		return fmt.Errorf("failed to check if directory %s exists: %w", path, err)
	}

	// Directory doesn't exist, try to create it
	err = os.Mkdir(path, 0)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return nil
}

// EnsureInstallerDataDir creates/updates the root directory for the installer data and sets permissions
// to ensure that only Administrators have write access to the directory tree.
//
// bootstrap runs before the MSI, so it must create the directory with the correct permissions.
func EnsureInstallerDataDir() error {
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

	// check if DatadogDataDir exists
	_, err := os.Stat(DatadogDataDir)
	if errors.Is(err, fs.ErrNotExist) {
		// DatadogDataDir does not exist, so we need to create it
		// probably means the MSI has yet to run
		// we'll create the directory with the restricted permissions
		// the MSI will run and fix the permissions soon after
		err = createDirectoryWithSDDL(DatadogDataDir, sddl)
		if err != nil {
			return fmt.Errorf("failed to create DatadogDataDir: %w", err)
		}
	}

	return winio.RunWithPrivileges(privilegesRequired, func() error {
		// Create root path: `C:\ProgramData\Datadog\Installer`
		err := secureCreateDirectory(DatadogInstallerData, sddl)
		if err != nil {
			return fmt.Errorf("failed to create DatadogInstallerData: %w", err)
		}

		// The root directory now exists with the correct permissions
		// we still need to ensure the subdirectories have the correct permissions.

		// Create subdirectories that inherit permissions from the parent
		if err := createDirIfNotExists(RootTmpDir); err != nil {
			return err
		}
		err = resetPermissionsForTree(RootTmpDir)
		if err != nil {
			return fmt.Errorf("failed to reset permissions for RootTmpDir: %w", err)
		}

		// Create subdirectories that have different permissions (global read)
		// PackagesPath should only contain files from public OCI packages
		if err := createDirIfNotExists(PackagesPath); err != nil {
			return err
		}
		err = SetRepositoryPermissions(PackagesPath)
		if err != nil {
			return fmt.Errorf("failed to create PackagesPath: %w", err)
		}
		// ConfigsPath has generated configuration files but will not contain secrets.
		// To support options that are secrets, we will need to fetch them from a secret store.
		if err := createDirIfNotExists(ConfigsPath); err != nil {
			return err
		}
		err = SetRepositoryPermissions(ConfigsPath)
		if err != nil {
			return fmt.Errorf("failed to create ConfigsPath: %w", err)
		}

		return nil
	})
}

// secureCreateDirectory creates a directory with the specified SDDL string.
//
// If the directory already exists and it is owned by Administrators or SYSTEM, the permissions
// are set to the expected state. If the directory is owned by an unknown party, an error is returned.
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
	err = IsDirSecure(path)
	if err != nil {
		// The directory owner is not Administrators or SYSTEM, so may have been created
		// by an unknown party. Adjusting the permissions may not be safe, as it won't affect
		// already open handles, so we fail the install.
		return err
	}

	// The owner is Administrators or SYSTEM, so we can be reasonably sure the directory and its
	// original permissions were created by an Administrator. If the Administrator created
	// the directory insecurely, we'll reset the permissions here, but we can't account
	// for created/changed files in the tree during that time. The caller may opt to reset the
	// permissions recursively, but this is not done here as we have paths that have children
	// with different permissions.
	err = setNamedSecurityInfoWithSDDL(path, sddl)
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
	targetDir := DatadogInstallerData
	log.Infof("Checking if installer data directory is secure: %s", targetDir)
	return IsDirSecure(targetDir)
}

// IsDirSecure returns nil if the directory is owned by Administrators or SYSTEM,
// otherwise an error is returned.
func IsDirSecure(targetDir string) error {
	allowedWellKnownSids := []windows.WELL_KNOWN_SID_TYPE{
		windows.WinBuiltinAdministratorsSid,
		windows.WinLocalSystemSid,
	}

	// get security info
	sd, err := windows.GetNamedSecurityInfo(targetDir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("failed to get security info for dir \"%s\": %w", targetDir, err)
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

func setNamedSecurityInfoWithSDDL(root string, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	return setNamedSecurityInfoFromSecurityDescriptor(root, sd)
}

func treeResetNamedSecurityInfoWithSDDL(root string, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	return treeResetNamedSecurityInfoFromSecurityDescriptor(root, sd)
}

func getSecurityInfoFromSecurityDescriptor(sd *windows.SECURITY_DESCRIPTOR) (*securityInfo, error) {
	var flags windows.SECURITY_INFORMATION
	control, _, err := sd.Control()
	if err != nil {
		return nil, err
	}
	flags |= securityInformationFromControlFlags(control)

	owner, _, err := sd.Owner()
	if err != nil {
		return nil, err
	}
	if owner != nil {
		flags |= windows.OWNER_SECURITY_INFORMATION
	}
	group, _, err := sd.Group()
	if err != nil {
		return nil, err
	}
	if group != nil {
		flags |= windows.GROUP_SECURITY_INFORMATION
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		if err != windows.ERROR_OBJECT_NOT_FOUND {
			return nil, err
		}
	} else {
		flags |= windows.DACL_SECURITY_INFORMATION
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		if err != windows.ERROR_OBJECT_NOT_FOUND {
			return nil, err
		}
	} else {
		flags |= windows.SACL_SECURITY_INFORMATION
	}
	return &securityInfo{
		Flags: flags,
		Owner: owner,
		Group: group,
		DACL:  dacl,
		SACL:  sacl,
	}, nil
}

func setNamedSecurityInfoFromSecurityDescriptor(root string, sd *windows.SECURITY_DESCRIPTOR) error {
	info, err := getSecurityInfoFromSecurityDescriptor(sd)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, info.Flags, info.Owner, info.Group, info.DACL, info.SACL)
}

func treeResetNamedSecurityInfoFromSecurityDescriptor(root string, sd *windows.SECURITY_DESCRIPTOR) error {
	info, err := getSecurityInfoFromSecurityDescriptor(sd)
	if err != nil {
		return err
	}
	err = TreeResetNamedSecurityInfo(
		root,
		windows.SE_FILE_OBJECT,
		info.Flags,
		info.Owner,
		info.Group,
		info.DACL,
		info.SACL,
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

// getProgramDataDirForProduct returns the current programdatadir, usually
// c:\programdata\Datadog given a product key name
func getProgramDataDirForProduct(product string) (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		// Something is terribly wrong on the system if %PROGRAMDATA% is missing
		return "", err
	}
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", keyname)
		return filepath.Join(res, "Datadog"), nil
	}
	defer k.Close()
	val, _, err := k.GetStringValue("ConfigRoot")
	if err != nil {
		log.Warnf("Windows installation key config not found, using default program data dir")
		return filepath.Join(res, "Datadog"), nil
	}
	path = val
	return
}

// getProgramFilesDirForProduct returns the root of the installatoin directory,
// usually c:\program files\datadog\datadog agent
func getProgramFilesDirForProduct(product string) (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		// Something is terribly wrong on the system if %PROGRAMFILES% is missing
		return "", err
	}
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", keyname)
		return filepath.Join(res, "Datadog", product), nil
	}
	defer k.Close()
	val, _, err := k.GetStringValue("InstallPath")
	if err != nil {
		log.Warnf("Windows installation key config not found, using default program data dir")
		return filepath.Join(res, "Datadog", product), nil
	}
	path = val
	return
}

// SetRepositoryPermissions sets the permissions on the repository directory
// It needs to be world readable so that user processes can load installed libraries
func SetRepositoryPermissions(path string) error {
	// Desired permissions:
	// - OWNER: Administrators
	// - GROUP: Administrators
	// - SYSTEM: Full Control (propagates to children)
	// - Administrators: Full Control (propagates to children)
	// - Everyone: 0x1200A9 Read and execute (propagates to children)
	// - PROTECTED: does not inherit permissions from parent
	sddl := "O:BAG:BAD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200A9;;;WD)"

	return treeResetNamedSecurityInfoWithSDDL(path, sddl)
}

// GetAdminInstallerBinaryPath returns the path to the datadog-installer executable
// inside an MSI administrative install extracted directory tree.
//
// https://learn.microsoft.com/en-us/windows/win32/msi/administrative-installation
func GetAdminInstallerBinaryPath(path string) string {
	return filepath.Join(path, "ProgramFiles64Folder", "Datadog", "Datadog Agent", "bin", "datadog-installer.exe")
}

// resetPermissionsForTree sets the owner/group to Administrators, enables inheritance, and removes all explicit ACEs.
func resetPermissionsForTree(path string) error {
	// set owner/group to Administrators
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	// set owner, group, and disable protection (enable inheritance) on the DACL
	var flags windows.SECURITY_INFORMATION
	flags |= windows.OWNER_SECURITY_INFORMATION
	flags |= windows.GROUP_SECURITY_INFORMATION
	flags |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	return TreeResetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, flags, admins, admins, nil, nil, false)
}
