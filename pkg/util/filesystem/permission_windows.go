// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package filesystem

import (
	"fmt"

	"github.com/hectane/go-acl"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// Permission handles permissions for Unix and Windows
type Permission struct {
	ddUserSid        *windows.SID
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

	currentUserSid, err := getDataDogUserSid()
	if err != nil {
		return nil, fmt.Errorf("Unable to get datadog user sid %v", err)
	}
	return &Permission{
		ddUserSid:        currentUserSid,
		administratorSid: administratorSid,
		systemSid:        systemSid,
	}, nil
}

var getDataDogUserSid = func() (*windows.SID, error) {
	return winutil.GetDDAgentUserSID()
}

// RestrictAccessToUser update the ACL of a file so only the current user and ADMIN/SYSTEM can access it
func (p *Permission) RestrictAccessToUser(path string) error {
	return acl.Apply(
		path,
		true,  // replace the file permissions
		false, // don't inherit
		acl.GrantSid(windows.GENERIC_ALL, p.administratorSid),
		acl.GrantSid(windows.GENERIC_ALL, p.systemSid),
		acl.GrantSid(windows.GENERIC_ALL, p.ddUserSid))
}

// RemoveAccessToOtherUsers on Windows this function calls RestrictAccessToUser
func (p *Permission) RemoveAccessToOtherUsers(path string) error {
	return p.RestrictAccessToUser(path)
}
