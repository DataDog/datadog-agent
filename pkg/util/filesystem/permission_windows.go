// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package filesystem

import (
	"fmt"
	"syscall"

	"github.com/hectane/go-acl"
	"golang.org/x/sys/windows"
)

// Permission handles permissions for Unix and Windows
type Permission struct {
	currentUserSid   *windows.SID
	administratorSid *windows.SID
	systemSid        *windows.SID
}

// NewPermission creates a new instance of `Permission`
func NewPermission() (*Permission, error) {
	administratorSid, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return nil, err
	}
	systemSid, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return nil, err
	}

	currentUserSid, err := getCurrentUserSid()
	if err != nil {
		return nil, fmt.Errorf("Unable to get current user sid %v", err)
	}
	return &Permission{
		currentUserSid:   currentUserSid,
		administratorSid: administratorSid,
		systemSid:        systemSid,
	}, nil
}

func getCurrentUserSid() (*windows.SID, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return nil, fmt.Errorf("Couldn't get process token %v", err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("Couldn't get token user %v", err)
	}
	sidString, err := user.User.Sid.String()
	if err != nil {
		return nil, fmt.Errorf("Couldn't get user sid string %v", err)
	}
	return windows.StringToSid(sidString)
}

// RestrictAccessToUser update the ACL of a file so only the current user and ADMIN/SYSTEM can access it
func (p *Permission) RestrictAccessToUser(path string) error {
	return acl.Apply(
		path,
		true,  // replace the file permissions
		false, // don't inherit
		acl.GrantSid(windows.GENERIC_ALL, p.administratorSid),
		acl.GrantSid(windows.GENERIC_ALL, p.systemSid),
		acl.GrantSid(windows.GENERIC_ALL, p.currentUserSid))
}

// RemoveAccessToOtherUsers on Windows this function calls RestrictAccessToUser
func (p *Permission) RemoveAccessToOtherUsers(path string) error {
	return p.RestrictAccessToUser(path)
}
