// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var warnOnce sync.Once

// Permission handles permissions for Unix and Windows
type Permission struct{}

// NewPermission creates a new instance of `Permission`
func NewPermission() (*Permission, error) {
	return &Permission{}, nil
}

// return the user dd-agent uid and gid
func UserDDAgent() (usrID int, grpID int, err error) {
	usr, err := user.Lookup("dd-agent")
	if err != nil {
		return 0, 0, err
	}

	usrID, err = strconv.Atoi(usr.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("couldn't parse UID (%s): %w", usr.Uid, err)
	}

	grpID, err = strconv.Atoi(usr.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("couldn't parse GID (%s): %w", usr.Gid, err)
	}
	return usrID, grpID, nil
}

// RestrictAccessToUser sets the file user and group to the same as 'dd-agent' user. If the function fails to lookup
// "dd-agent" user it return nil immediately.
func (p *Permission) RestrictAccessToUser(path string) error {
	usrID, grpID, err := UserDDAgent()
	if err != nil {
		warnOnce.Do(func() {
			log.Warnf("dd-agent user not found, skipping restriction")
		})
		return nil
	}
	if err = os.Chown(path, usrID, grpID); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			log.Infof("Cannot change owner of '%s', permission denied", path)
			return nil
		}

		return fmt.Errorf("couldn't set user and group owner for %s: %w", path, err)
	}

	return nil
}

// RemoveAccessToOtherUsers on Unix this calls RestrictAccessToUser and then removes all access to the file for 'group'
// and 'other'
func (p *Permission) RemoveAccessToOtherUsers(path string) error {
	// We first try to set other and group to "dd-agent" when possible
	_ = p.RestrictAccessToUser(path)

	fperm, err := os.Stat(path)
	if err != nil {
		return err
	}
	// We keep the original 'user' rights but set 'group' and 'other' to zero.
	newPerm := fperm.Mode().Perm() & 0700
	return os.Chmod(path, fs.FileMode(newPerm))
}
