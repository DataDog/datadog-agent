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

	//revive:disable:var-naming Name is intended to match the Windows API name
	procGetAclInformation    = advapi32.NewProc("GetAclInformation")
	procGetNamedSecurityInfo = advapi32.NewProc("GetNamedSecurityInfoW")
	procGetAce               = advapi32.NewProc("GetAce")
	//revive:enable:var-naming
)

// ACL_SIZE_INFORMATION struct
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-acl_size_information
//
//revive:disable:var-naming Name is intended to match the Windows type name
type ACL_SIZE_INFORMATION struct {
	AceCount      uint32
	AclBytesInUse uint32
	AclBytesFree  uint32
}

// ACL struct
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-acl
type ACL struct {
	AclRevision uint8
	Sbz1        uint8
	AclSize     uint16
	AceCount    uint16
	Sbz2        uint16
}

// ACCESS_ALLOWED_ACE struct
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-access_allowed_ace
type ACCESS_ALLOWED_ACE struct {
	AceType    uint8
	AceFlags   uint8
	AceSize    uint16
	AccessMask uint32
	SidStart   uint32
}

//revive:enable:var-naming (types)

//revive:disable:var-naming Name is intended to match the Windows const name

// ACL_INFORMATION_CLASS enum
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ne-winnt-acl_information_class
const (
	AclRevisionInformation = 1
	AclSizeInformation     = 2
)

// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-ace_header
const (
	ACCESS_ALLOWED_ACE_TYPE = 0
	ACCESS_DENIED_ACE_TYPE  = 1
)

//revive:enable:var-naming (const)

// GetAclInformation calls windows 'GetAclInformation' function to retrieve
// information about an access control list (ACL).
//
// https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-getaclinformation
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func GetAclInformation(acl *ACL, info *ACL_SIZE_INFORMATION, class uint32) error {
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
//
// https://learn.microsoft.com/en-us/windows/win32/api/aclapi/nf-aclapi-getnamedsecurityinfow
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func GetNamedSecurityInfo(objectName string, objectType int32, secInfo uint32, owner, group **windows.SID, dacl, sacl **ACL, secDesc *windows.Handle) error {
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
//
// https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-getace
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func GetAce(acl *ACL, index uint32, ace **ACCESS_ALLOWED_ACE) error {
	ret, _, _ := procGetAce.Call(uintptr(unsafe.Pointer(acl)), uintptr(index), uintptr(unsafe.Pointer(ace)))
	if int(ret) != 0 {
		return windows.GetLastError()
	}
	return nil
}
