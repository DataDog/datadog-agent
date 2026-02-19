// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"errors"
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

	ddUserSid, err := getDatadogUserSid()
	if err != nil {
		return nil, fmt.Errorf("unable to get datadog user sid %v", err)
	}
	return &Permission{
		ddUserSid:        ddUserSid,
		administratorSid: administratorSid,
		systemSid:        systemSid,
	}, nil
}

func getDatadogUserSid() (*windows.SID, error) {
	ddUserSid, err := winutil.GetDDAgentUserSID()
	if err != nil {
		// falls back to current user on failure
		return winutil.GetSidFromUser()
	}
	return ddUserSid, nil
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

func (p *Permission) isAllowedOwner(sid *windows.SID) bool {
	return windows.EqualSid(sid, p.administratorSid) ||
		windows.EqualSid(sid, p.ddUserSid) ||
		windows.EqualSid(sid, p.systemSid)
}

// checkOwner verifies that the file/directory is owned by either 'Administrator', system or dd user
func (p *Permission) checkOwner(path string) error {
	var ownerSid *windows.SID
	err := winutil.GetNamedSecurityInfo(path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
		&ownerSid,
		nil, nil, nil, nil)

	if err != nil {
		return err
	}

	if !p.isAllowedOwner(ownerSid) {
		return errors.New("file owner is neither `Administrator`, system or dd user")
	}

	return nil
}

// CheckOwnerAndPermissions verifies that the owner and permissions of a file/directory correspond to the expected restrictions
func CheckOwnerAndPermissions(path string) error {
	p, err := NewPermission()
	if err != nil {
		return err
	}

	if err := p.checkOwner(path); err != nil {
		return err
	}

	return CheckRights(path, false)
}
