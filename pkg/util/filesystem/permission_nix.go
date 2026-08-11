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
	"runtime"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Permission handles permissions for Unix and Windows
type Permission struct {
	ddUserUID uint32
}

// NewPermission creates a new instance of `Permission`
func NewPermission() (*Permission, error) {
	perms := &Permission{}

	ddUserUID, err := getDatadogUserUID()
	if err != nil {
		return perms, err
	}

	perms.ddUserUID = ddUserUID
	return perms, nil
}

// agentUsername returns the agent user name for the current platform.
// macOS uses "_dd-agent" (underscore prefix is the convention for daemon accounts),
// Linux uses "dd-agent".
func agentUsername() string {
	if runtime.GOOS == "darwin" {
		return "_dd-agent"
	}
	return "dd-agent"
}

// agentUserIDs returns the agent user's uid and gid. found is false when the
// agent user does not exist on this host, which callers treat as "nothing to do"
// rather than as an error.
func agentUserIDs() (uid int, gid int, found bool, err error) {
	usr, err := user.Lookup(agentUsername())
	if err != nil {
		return 0, 0, false, nil
	}

	uid, err = strconv.Atoi(usr.Uid)
	if err != nil {
		return 0, 0, false, fmt.Errorf("couldn't parse UID (%s): %w", usr.Uid, err)
	}

	gid, err = strconv.Atoi(usr.Gid)
	if err != nil {
		return 0, 0, false, fmt.Errorf("couldn't parse GID (%s): %w", usr.Gid, err)
	}

	return uid, gid, true, nil
}

// RestrictAccessToUser sets the file user and group to the same as the agent
// user. On Linux this is "dd-agent"; on macOS it is "_dd-agent". If neither
// user exists, the function returns nil immediately.
func (p *Permission) RestrictAccessToUser(path string) error {
	usrID, grpID, found, err := agentUserIDs()
	if err != nil {
		return err
	}
	if !found {
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

// SetAgentGroupOwnerNoFollow sets the file's group owner to the Agent user's
// group, leaving the user owner untouched, and does not follow symlinks. If the
// Agent user does not exist, the function returns nil immediately.
//
// Note that this *grants* the Agent user's group whatever the file's group bits
// allow, unlike the Restrict* helpers above. Prefer it over RestrictAccessToUser
// for a path inside a directory an unprivileged user can write to: chown(2)
// follows symlinks, so between creating a file and chowning it by path, a process
// that can write the directory can substitute a symlink and have the privileged
// process hand it a file of its choosing. Changing only the group also keeps the
// file owned by the caller.
//
// It is a plain function rather than a method on Permission because it needs
// nothing from the receiver, and it is unix-only: Windows has no equivalent.
func SetAgentGroupOwnerNoFollow(path string) error {
	_, grpID, found, err := agentUserIDs()
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	// -1 leaves the user owner as it is.
	if err := os.Lchown(path, -1, grpID); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			log.Infof("Cannot change group of '%s', permission denied", path)
			return nil
		}

		return fmt.Errorf("couldn't set group owner for %s: %w", path, err)
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

func getDatadogUserUID() (uint32, error) {
	if ddAgentUser, err := user.Lookup(agentUsername()); err == nil {
		ddAgentUID, err := strconv.Atoi(ddAgentUser.Uid)
		if err != nil {
			return 0, err
		}
		return uint32(ddAgentUID), nil
	}

	// agent user not found, fall back to the current user
	return uint32(os.Getuid()), nil
}

// isRootOrAgentUID reports whether uid is root (0) or the dd-agent service account.
func (p *Permission) isRootOrAgentUID(uid uint32) bool {
	return uid == 0 || uid == p.ddUserUID
}

// checkOwner verifies that path is owned by root or dd-agent.
func (p *Permission) checkOwner(path string) error {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return err
	}

	if !p.isRootOrAgentUID(stat.Uid) {
		return errors.New("file owner is not a trusted user")
	}

	return nil
}
