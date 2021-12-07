// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build !windows

package filesystem

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
)

// Permission handles permissions for Unix and Windows
type Permission struct{}

// NewPermission creates a new instance of `Permission`
func NewPermission() (*Permission, error) {
	return &Permission{}, nil
}

// RestrictAccessToUser restricts the access to the user (chmod 700)
func (p *Permission) RestrictAccessToUser(path string) error {
	usr, err := user.Lookup("dd-agent")
	if err == nil {
		usrID, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("couldn't parse UID (%s): %w", usr.Uid, err)
		}

		grpID, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return fmt.Errorf("couldn't parse GID (%s): %w", usr.Gid, err)
		}

		if err = os.Chown(path, usrID, grpID); err != nil {
			return fmt.Errorf("couldn't set user and group owner for %s: %w", path, err)
		}
	}

	return nil
}
