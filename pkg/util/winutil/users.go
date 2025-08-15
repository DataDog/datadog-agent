// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.
//go:build windows

package winutil

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#cgo LDFLAGS: -lnetapi32 -ladvapi32
#include "users.h"
*/
import "C"

// Common Windows error codes and NTSTATUS values
const (
	// NTSTATUS values (keep these as they're not in public golang.org/x/sys/windows)
	StatusSuccess               = 0x00000000
	StatusAccessDenied          = 0xC0000022
	StatusObjectNameNotFound    = 0xC0000034
	StatusInsufficientResources = 0xC000009A
	StatusNoSuchPrivilege       = 0xC0000060

	// Network API error codes (keep these as they're specific to NetAPI)
	NerrUserNotFound  = 2221
	NerrInternalError = 2140

	// Custom error codes for DLL availability (from users.c)
	ErrorDllNotAvailable = 0x80070002 // ERROR_FILE_NOT_FOUND - DLL not available (e.g., Windows Nano)
)

// Use standard Windows constants from golang.org/x/sys/windows for common Win32 errors
// windows.ERROR_ACCESS_DENIED = 5
// windows.ERROR_NOT_ENOUGH_MEMORY = 8
// windows.ERROR_INVALID_PARAMETER = 87

// GetSidFromUser grabs and returns the windows SID for the current user or an error.
// The *SID returned does not need to be freed by the caller.
func GetSidFromUser() (*windows.SID, error) {
	log.Infof("Getting sidstring from user")
	tok, e := syscall.OpenCurrentProcessToken()
	if e != nil {
		log.Warnf("Couldn't get process token %v", e)
		return nil, e
	}
	defer tok.Close()

	user, e := tok.GetTokenUser()
	if e != nil {
		log.Warnf("Couldn't get token user %v", e)
		return nil, e
	}

	sidString, e := user.User.Sid.String()
	if e != nil {
		log.Warnf("Couldn't get user sid string %v", e)
		return nil, e
	}

	return windows.StringToSid(sidString)
}

// IsUserAnAdmin returns true is a user is a member of the Administrator's group
//
// https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-isuseranadmin
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func IsUserAnAdmin() (bool, error) {
	var administratorsGroup *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&administratorsGroup)
	if err != nil {
		return false, fmt.Errorf("could not get Administrators group SID: %w", err)
	}
	defer windows.FreeSid(administratorsGroup)

	// call CheckTokenMembership to determine if the current user is a member of the administrators group
	var isAdmin bool
	err = CheckTokenMembership(0, administratorsGroup, &isAdmin)
	if err != nil {
		return false, fmt.Errorf("could not check token membership: %w", err)
	}

	return isAdmin, nil

}

// GetLocalSystemSID returns the SID of the Local System account
// the returned SID must be freed by windows.FreeSid()
func GetLocalSystemSID() (*windows.SID, error) {
	var localSystem *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		1, // local system has 1 valid subauth
		windows.SECURITY_LOCAL_SYSTEM_RID,
		0, 0, 0, 0, 0, 0, 0,
		&localSystem)

	return localSystem, err
}

// GetServiceUserSID returns the SID of the specified service account
func GetServiceUserSID(service string) (*windows.SID, error) {
	// get config for datadogagent service
	user, err := GetServiceUser(service)
	if err != nil {
		return nil, fmt.Errorf("could not get datadogagent service user: %s", err)
	}

	username, err := getUserFromServiceUser(user)
	if err != nil {
		return nil, err
	}

	// Manually map some aliases that SCM uses and are not recognized by the
	// security subsystem (`LookupAccountName()` will fail)
	// https://learn.microsoft.com/en-us/windows/win32/services/service-user-accounts
	if username == "LocalSystem" {
		return windows.StringToSid("S-1-5-18")
	}

	// get the SID for the user account
	sid, _, _, err := windows.LookupSID("", username)
	return sid, err
}

func getUserFromServiceUser(user string) (string, error) {
	var domain, username string
	parts := strings.SplitN(user, "\\", 2)
	if len(parts) == 1 {
		username = user
	} else if len(parts) == 2 {
		domain = parts[0]
		if domain == "." {
			username = parts[1]
		} else {
			username = user
		}
	} else {
		return "", fmt.Errorf("could not parse user: %s", user)
	}

	return username, nil

}

// GetDDAgentUserSID returns the SID of the DataDog Agent account
func GetDDAgentUserSID() (*windows.SID, error) {
	return GetServiceUserSID("datadogagent")
}

// GetDDUserGroups retrieves the local groups that the DDAgent user is a member of
func GetDDUserGroups() ([]string, error) {
	// Get the DDAgent username using service lookup
	user, err := GetServiceUser("datadogagent")
	if err != nil {
		return nil, fmt.Errorf("could not get datadogagent service user: %w", err)
	}

	// Process username to handle domain formats properly
	username, err := getUserFromServiceUser(user)
	if err != nil {
		return nil, err
	}

	// For local accounts, use computer name prefix instead of "." to avoid LookupAccountNameW issues
	if !strings.Contains(username, "\\") {
		computerName, err := windows.ComputerName()
		if err == nil {
			username = computerName + "\\" + username
		}
	}

	// Convert username to C string
	cUsername := C.CString(username)
	defer C.free(unsafe.Pointer(cUsername))

	// Call C function to get groups with error code
	var errorCode C.int
	cGroups := C.getLocalUserGroups(cUsername, &errorCode)
	if cGroups == nil {
		return nil, handleUserError("get user groups", username, int(errorCode))
	}
	defer C.free(unsafe.Pointer(cGroups))

	// Convert C string to Go string
	groupsStr := C.GoString(cGroups)

	// Split comma-separated string into slice
	if groupsStr == "" {
		return []string{}, nil
	}

	groups := strings.Split(groupsStr, ",")
	return groups, nil
}

// DoesAgentUserHaveDesiredGroups checks if the DD user has desired groups:
// - Event Log Readers
// - Performance Log Users
// - Performance Monitor Users
// Returns: (actualGroups, hasAllDesiredGroups, error)
func DoesAgentUserHaveDesiredGroups() ([]string, bool, error) {
	groups, err := GetDDUserGroups()
	if err != nil {
		return nil, false, fmt.Errorf("could not get DDAgent user groups: %w", err)
	}

	hasEventLogReaders := false
	hasPerformanceLogUsers := false
	hasPerformanceMonitorUsers := false

	// check if the groups contain the desired groups
	for _, group := range groups {
		if group == "Event Log Readers" {
			hasEventLogReaders = true
		}
		if group == "Performance Log Users" {
			hasPerformanceLogUsers = true
		}
		if group == "Performance Monitor Users" {
			hasPerformanceMonitorUsers = true
		}
	}

	hasAllDesired := hasEventLogReaders && hasPerformanceLogUsers && hasPerformanceMonitorUsers
	return groups, hasAllDesired, nil
}

// GetDDUserRights retrieves the account rights that the DDAgent user has
func GetDDUserRights() ([]string, error) {
	// Get the DDAgent username using service lookup
	user, err := GetServiceUser("datadogagent")
	if err != nil {
		return nil, fmt.Errorf("could not get datadogagent service user: %w", err)
	}

	// Process username to handle domain formats properly
	username, err := getUserFromServiceUser(user)
	if err != nil {
		return nil, err
	}

	// For local accounts, use computer name prefix instead of "." to avoid LookupAccountNameW issues
	if !strings.Contains(username, "\\") {
		computerName, err := windows.ComputerName()
		if err == nil {
			username = computerName + "\\" + username
		}
	}

	// Convert username to C string
	cUsername := C.CString(username)
	defer C.free(unsafe.Pointer(cUsername))

	// Call C function to get rights with error code
	var errorCode C.int
	cRights := C.getLocalAccountRights(cUsername, &errorCode)
	if cRights == nil {
		return nil, handleUserError("get user rights", username, int(errorCode))
	}
	defer C.free(unsafe.Pointer(cRights))

	// Convert C string to Go string
	rightsStr := C.GoString(cRights)

	// Split comma-separated string into slice
	if rightsStr == "" {
		return []string{}, nil
	}

	rights := strings.Split(rightsStr, ",")
	return rights, nil
}

// DoesAgentUserHaveDesiredRights checks if agent user account has desired rights:
// - SeServiceLogonRight
// - SeDenyInteractiveLogonRight
// - SeDenyNetworkLogonRight
// - SeDenyRemoteInteractiveLogonRight
// Returns: (actualRights, hasAllDesiredRights, error)
func DoesAgentUserHaveDesiredRights() ([]string, bool, error) {
	rights, err := GetDDUserRights()
	if err != nil {
		return nil, false, fmt.Errorf("could not get DDAgent user rights: %w", err)
	}

	hasSeServiceLogonRight := false
	hasSeDenyInteractiveLogonRight := false
	hasSeDenyNetworkLogonRight := false
	hasSeDenyRemoteInteractiveLogonRight := false

	// check if the rights contain the desired rights
	for _, right := range rights {
		if right == "SeServiceLogonRight" {
			hasSeServiceLogonRight = true
		}
		if right == "SeDenyInteractiveLogonRight" {
			hasSeDenyInteractiveLogonRight = true
		}
		if right == "SeDenyNetworkLogonRight" {
			hasSeDenyNetworkLogonRight = true
		}
		if right == "SeDenyRemoteInteractiveLogonRight" {
			hasSeDenyRemoteInteractiveLogonRight = true
		}
	}

	hasAllDesired := hasSeServiceLogonRight && hasSeDenyInteractiveLogonRight && hasSeDenyNetworkLogonRight && hasSeDenyRemoteInteractiveLogonRight
	return rights, hasAllDesired, nil
}

// handleUserError provides detailed error messages based on Windows error codes
// NTSTATUS: https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-erref/87fba13e-bf06-450e-83b1-9241dc81e781
func handleUserError(operation, username string, errorCode int) error {
	switch uint32(errorCode) {
	case ErrorDllNotAvailable:
		// Return an error that can be detected by diagnosis system as a warning (like access denied)
		return fmt.Errorf("Required Windows APIs not available for %s operation (user '%s'): likely running on systems such as Windows Nano Server", operation, username)

	case StatusAccessDenied:
		return fmt.Errorf("access denied while trying to %s for user '%s': administrator privileges may be required", operation, username)

	case StatusObjectNameNotFound, NerrUserNotFound:
		return fmt.Errorf("user '%s' not found while trying to %s", username, operation)

	case StatusInsufficientResources, uint32(windows.ERROR_NOT_ENOUGH_MEMORY):
		return fmt.Errorf("insufficient memory while trying to %s for user '%s'", operation, username)

	case StatusNoSuchPrivilege:
		return fmt.Errorf("no privileges found for user '%s' while trying to %s", username, operation)

	case uint32(windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("access denied while trying to %s for user '%s': administrator privileges may be required", operation, username)

	case uint32(windows.ERROR_INVALID_PARAMETER):
		return fmt.Errorf("invalid parameter while trying to %s for user '%s'", operation, username)

	default:
		// NTSTATUS codes are typically displayed in hex
		if errorCode < 0 {
			return fmt.Errorf("failed to %s for user '%s': NTSTATUS 0x%08X", operation, username, uint32(errorCode))
		}
		return fmt.Errorf("failed to %s for user '%s': Win32 error %d", operation, username, errorCode)
	}
}
