// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.
//go:build windows

package winutil

import (
	"fmt"
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
