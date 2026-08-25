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
	"path/filepath"
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

// RestrictAccessToUser sets the file user and group to the same as the agent
// user. On Linux this is "dd-agent"; on macOS it is "_dd-agent". If neither
// user exists, the function returns nil immediately.
func (p *Permission) RestrictAccessToUser(path string) error {
	usr, err := user.Lookup(agentUsername())
	if err != nil {
		return nil
	}

	usrID, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return fmt.Errorf("couldn't parse UID (%s): %w", usr.Uid, err)
	}

	grpID, err := strconv.Atoi(usr.Gid)
	if err != nil {
		return fmt.Errorf("couldn't parse GID (%s): %w", usr.Gid, err)
	}

	dir, name, err := splitDirBase(path)
	if err != nil {
		return fmt.Errorf("couldn't restrict access to %s: %w", path, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("couldn't open parent directory of %s: %w", path, err)
	}
	defer root.Close()

	if err = root.Lchown(name, usrID, grpID); err != nil {
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

	dir, name, err := splitDirBase(path)
	if err != nil {
		return fmt.Errorf("couldn't restrict access to %s: %w", path, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("couldn't open parent directory of %s: %w", path, err)
	}
	defer root.Close()

	fperm, err := root.Lstat(name)
	if err != nil {
		return err
	}
	// We keep the original 'user' rights but set 'group' and 'other' to zero.
	newPerm := fperm.Mode().Perm() & 0700
	return root.Chmod(name, fs.FileMode(newPerm))
}

// splitDirBase returns the parent directory and base name of path, with any
// trailing separators removed so that e.g. "/tmp/work/" splits into
// "/tmp" and "work" instead of "/tmp/work" and "work". An empty path
// is rejected, matching the error behavior of the previous os.Chown/Chmod.
func splitDirBase(path string) (dir, base string, err error) {
	if path == "" {
		return "", "", errors.New("path is empty")
	}
	cleaned := filepath.Clean(path)
	return filepath.Dir(cleaned), filepath.Base(cleaned), nil
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

// GetAgentUserGroupIDs returns the UID and primary GID of the agent service
// account ("dd-agent" on Linux, "_dd-agent" on macOS). The GID is the
// user's primary group (e.g. "root" in the standard container, where no
// same-named group exists), not a separately-named group. ok is false if the
// user does not exist on the system.
func GetAgentUserGroupIDs() (uid, gid int, ok bool) {
	u, err := user.Lookup(agentUsername())
	if err != nil {
		return 0, 0, false
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, false
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, false
	}
	return uid, gid, true
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
