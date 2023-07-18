// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build windows

package winutil

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procGetAclInformation    = advapi32.NewProc("GetAclInformation")
	procGetNamedSecurityInfo = advapi32.NewProc("GetNamedSecurityInfoW")
	procGetAce               = advapi32.NewProc("GetAce")
)

type AclSizeInformation struct {
	AceCount      uint32
	AclBytesInUse uint32
	AclBytesFree  uint32
}

type Acl struct {
	AclRevision uint8
	Sbz1        uint8
	AclSize     uint16
	AceCount    uint16
	Sbz2        uint16
}

type AccessAllowedAce struct {
	AceType    uint8
	AceFlags   uint8
	AceSize    uint16
	AccessMask uint32
	SidStart   uint32
}

const (
	AclRevisionInformationEnum = 1
	AclSizeInformationEnum     = 2
)

const (
	ACCESS_ALLOWED_ACE_TYPE = 0
	ACCESS_DENIED_ACE_TYPE  = 1
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379593.aspx
const (
	SE_UNKNOWN_OBJECT_TYPE = iota
	SE_FILE_OBJECT
	SE_SERVICE
	SE_PRINTER
	SE_REGISTRY_KEY
	SE_LMSHARE
	SE_KERNEL_OBJECT
	SE_WINDOW_OBJECT
	SE_DS_OBJECT
	SE_DS_OBJECT_ALL
	SE_PROVIDER_DEFINED_OBJECT
	SE_WMIGUID_OBJECT
	SE_REGISTRY_WOW64_32KEY
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/aa379573.aspx
const (
	OWNER_SECURITY_INFORMATION               = 0x00001
	GROUP_SECURITY_INFORMATION               = 0x00002
	DACL_SECURITY_INFORMATION                = 0x00004
	SACL_SECURITY_INFORMATION                = 0x00008
	LABEL_SECURITY_INFORMATION               = 0x00010
	ATTRIBUTE_SECURITY_INFORMATION           = 0x00020
	SCOPE_SECURITY_INFORMATION               = 0x00040
	PROCESS_TRUST_LABEL_SECURITY_INFORMATION = 0x00080
	BACKUP_SECURITY_INFORMATION              = 0x10000

	PROTECTED_DACL_SECURITY_INFORMATION   = 0x80000000
	PROTECTED_SACL_SECURITY_INFORMATION   = 0x40000000
	UNPROTECTED_DACL_SECURITY_INFORMATION = 0x20000000
	UNPROTECTED_SACL_SECURITY_INFORMATION = 0x10000000
)

// GetAclInformation calls windows 'GetAclInformation' function to retrieve
// information about an access control list (ACL).
func GetAclInformation(acl *Acl, info *AclSizeInformation, class uint32) error {
	length := unsafe.Sizeof(*info)
	ret, _, _ := procGetAclInformation.Call(
		uintptr(unsafe.Pointer(acl)),
		uintptr(unsafe.Pointer(info)),
		uintptr(length),
		uintptr(class))

	if int(ret) == 0 {
		return windows.GetLastError()
	}
	return nil
}

// GetNamedSecurityInfo calls Windows 'GetNamedSecurityInfo' function to
// retrieve a copy of the security descriptor for an object specified by name.
func GetNamedSecurityInfo(objectName string, objectType int32, secInfo uint32, owner, group **windows.SID, dacl, sacl **Acl, secDesc *windows.Handle) error {
	ret, _, err := procGetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(objectName))),
		uintptr(objectType),
		uintptr(secInfo),
		uintptr(unsafe.Pointer(owner)),
		uintptr(unsafe.Pointer(group)),
		uintptr(unsafe.Pointer(dacl)),
		uintptr(unsafe.Pointer(sacl)),
		uintptr(unsafe.Pointer(secDesc)),
	)
	if ret != 0 {
		return err
	}
	return nil
}

// GetAce calls Windows 'GetAce' function to obtain a pointer to an access
// control entry (ACE) in an access control list (ACL).
func GetAce(acl *Acl, index uint32, ace **AccessAllowedAce) error {
	ret, _, _ := procGetAce.Call(uintptr(unsafe.Pointer(acl)), uintptr(index), uintptr(unsafe.Pointer(ace)))
	if int(ret) != 0 {
		return windows.GetLastError()
	}
	return nil
}
