// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windowsuser

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	logonclidll                = windows.NewLazySystemDLL("logoncli.dll")
	procNetIsServiceAccount    = logonclidll.NewProc("NetIsServiceAccount")
	procNetQueryServiceAccount = logonclidll.NewProc("NetQueryServiceAccount")

	netapi32dll          = windows.NewLazySystemDLL("netapi32.dll")
	procNetApiBufferFree = netapi32dll.NewProc("NetApiBufferFree")

	advapi32dll                    = windows.NewLazySystemDLL("ADVAPI32.dll")
	procGetWindowsAccountDomainSid = advapi32dll.NewProc("GetWindowsAccountDomainSid")
)

// Windows status codes
//
//revive:disable:var-naming match Windows status code names
const (
	STATUS_OBJECT_NAME_NOT_FOUND = windows.NTStatus(0xC0000034)
)

// MSA_INFO_STATE
//
// https://learn.microsoft.com/en-us/windows/win32/api/lmaccess/ne-lmaccess-msa_info_state
const (
	MsaInfoNotExist      = 1
	MsaInfoNotService    = 2
	MsaInfoCannotInstall = 3
	MsaInfoCanInstall    = 4
	MsaInfoInstalled     = 5
)

// MSA_INFO_STATE enum
//
// https://learn.microsoft.com/en-us/windows/win32/api/lmaccess/ne-lmaccess-msa_info_state
type MSA_INFO_STATE int

//revive:enable:var-naming

// NetIsServiceAccount returns true if the account is a sMSA or gMSA.
//
// This function RPC connects to the local netlogon service, which is only
// running on domain joined machines. On standalone machines, an error is returned.
//
// If the account is not found in the local netlogon store, the function may try to
// contact a domain controller which requires network credentials. Some environments,
// such as WinRM, ansible, and ssh key authentication, do not have network credentials
// and this call will fail with STATUS_OPEN_FAILED (decimal -1073741514 / hex 0xc0000136).
// For more information, see the "double hop problem".
// Interestingly, this issue does not occur when this code runs as SYSTEM, because the
// computer credentials are accepted. This can be a valid workaround in ansible.
//
// This function returns an error for accounts with non-domain prefixes like NT AUTHORITY\SYSTEM
//
// NetIsServiceAccount returns true if NetQueryServiceAccount returns MsaInfoInstalled,
// this is the same behavior as the Test-ADServiceAccount cmdlet in PowerShell.
//
// https://learn.microsoft.com/en-us/windows/win32/api/lmaccess/nf-lmaccess-netisserviceaccount
func NetIsServiceAccount(username string) (bool, error) {
	u, err := windows.UTF16PtrFromString(username)
	if err != nil {
		return false, err
	}
	var isServiceAccountParam uint32
	r1, _, _ := procNetIsServiceAccount.Call(
		0,                          // server, 0 for local machine
		uintptr(unsafe.Pointer(u)), // username
		uintptr(unsafe.Pointer(&isServiceAccountParam)),
	)
	if r1 != 0 {
		return false, windows.NTStatus(r1)
	}
	return isServiceAccountParam != 0, nil
}

// GetWindowsAccountDomainSid returns a SID representing the domain of that SID
//
// For example:
//   - for local accounts, returns the local machine SID (LookupAccountName(hostname))
//   - for domain accounts, returns the domain SID
//
// For special sids, such as container users and LocalSystem, returns ERROR_NON_ACCOUNT_SID
//
// https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-getwindowsaccountdomainsid
func GetWindowsAccountDomainSid(sid *windows.SID) (*windows.SID, error) {
	var domainSidSize uint32
	r1, _, err := procGetWindowsAccountDomainSid.Call(
		uintptr(unsafe.Pointer(sid)),
		0, // NULL to request size
		uintptr(unsafe.Pointer(&domainSidSize)),
	)
	// returns false on error, check gle
	if r1 == 0 {
		if !errors.Is(err, windows.Errno(windows.ERROR_INSUFFICIENT_BUFFER)) {
			return nil, err
		}
	}
	b := make([]byte, domainSidSize)
	r1, _, err = procGetWindowsAccountDomainSid.Call(
		uintptr(unsafe.Pointer(sid)),
		uintptr(unsafe.Pointer(unsafe.SliceData(b))),
		uintptr(unsafe.Pointer(&domainSidSize)),
	)
	// returns false on error, check gle
	if r1 == 0 {
		return nil, err
	}
	return (*windows.SID)(unsafe.Pointer(unsafe.SliceData(b))).Copy()
}

// GetComputerName returns the NetBIOS name of the local computer.
//
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getcomputernamew
func GetComputerName() (string, error) {
	var computerName [windows.MAX_COMPUTERNAME_LENGTH + 1]uint16
	var size uint32 = windows.MAX_COMPUTERNAME_LENGTH + 1
	err := windows.GetComputerName(&computerName[0], &size)
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(computerName[:]), nil
}

// NetQueryServiceAccount returns the service account type of the account.
//
// See NetIsServiceAccount for more important usage details.
//
// https://learn.microsoft.com/en-us/windows/win32/api/lmaccess/nf-lmaccess-netqueryserviceaccount
func NetQueryServiceAccount(username string) (MSA_INFO_STATE, error) {
	u, err := windows.UTF16PtrFromString(username)
	if err != nil {
		return 0, err
	}
	var info *uint32
	r1, _, _ := procNetQueryServiceAccount.Call(
		0,                          // server, 0 for local machine
		uintptr(unsafe.Pointer(u)), // username
		0,                          // MSA_INFO_0
		uintptr(unsafe.Pointer(&info)),
	)
	if r1 != 0 {
		return 0, windows.NTStatus(r1)
	}
	defer procNetApiBufferFree.Call(uintptr(unsafe.Pointer(info))) //nolint:errcheck
	return MSA_INFO_STATE(*info), nil
}

func (m MSA_INFO_STATE) String() string {
	switch m {
	case MsaInfoNotExist:
		return "MsaInfoNotExist"
	case MsaInfoNotService:
		return "MsaInfoNotService"
	case MsaInfoCannotInstall:
		return "MsaInfoCannotInstall"
	case MsaInfoCanInstall:
		return "MsaInfoCanInstall"
	case MsaInfoInstalled:
		return "MsaInfoInstalled"
	}
	return "unknown"
}
