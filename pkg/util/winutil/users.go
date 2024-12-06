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

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
// TODO: Microsoft does not recommend using this function, instead CheckTokenMembership should be used.
//
// https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/nf-shlobj_core-isuseranadmin
//
//revive:disable-next-line:var-naming Name is intended to match the Windows API name
func IsUserAnAdmin() (bool, error) {
	shell32 := windows.NewLazySystemDLL("Shell32.dll")
	defer windows.FreeLibrary(windows.Handle(shell32.Handle()))

	isUserAnAdminProc := shell32.NewProc("IsUserAnAdmin")
	ret, _, winError := isUserAnAdminProc.Call()

	if winError != windows.NTE_OP_OK {
		return false, fmt.Errorf("IsUserAnAdmin returns error code %d", winError)
	}
	if ret == 0 {
		return false, nil
	}
	return true, nil
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
