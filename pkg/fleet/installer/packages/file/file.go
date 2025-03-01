// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package file offers filesystem utils geared towards idempotent operations.
package file

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/bmatcuk/doublestar/v4"
)

// Directory represents a desired state for a directory.
type Directory struct {
	Path  string
	Mode  os.FileMode
	Owner string
	Group string
}

// Directories is a collection of directories.
type Directories []Directory

// Ensure ensures that the directory is created with the desired permissions.
func (d Directory) Ensure() error {
	uid, gid, err := getUserAndGroup(d.Owner, d.Group)
	if err != nil {
		return fmt.Errorf("error getting user and group IDs: %w", err)
	}
	err = os.MkdirAll(d.Path, d.Mode)
	if err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}
	err = os.Chown(d.Path, uid, gid)
	if err != nil {
		return fmt.Errorf("error changing directory ownership: %w", err)
	}
	err = os.Chmod(d.Path, d.Mode)
	if err != nil {
		return fmt.Errorf("error changing directory mode: %w", err)
	}
	return nil
}

// Ensure ensures that the directories are created with the desired permissions.
func (ds Directories) Ensure() error {
	for _, d := range ds {
		if err := d.Ensure(); err != nil {
			return err
		}
	}
	return nil
}

func getUserAndGroup(username string, group string) (uid, gid int, err error) {
	ddAgentUser, err := user.Lookup(username)
	if err != nil {
		return -1, -1, fmt.Errorf("dd-agent user not found: %w", err)
	}
	ddAgentGroup, err := user.LookupGroup(group)
	if err != nil {
		return -1, -1, fmt.Errorf("dd-agent group not found: %w", err)
	}
	ddAgentUID, err := strconv.Atoi(ddAgentUser.Uid)
	if err != nil {
		return -1, -1, fmt.Errorf("error converting dd-agent UID to int: %w", err)
	}
	ddAgentGID, err := strconv.Atoi(ddAgentGroup.Gid)
	if err != nil {
		return -1, -1, fmt.Errorf("error converting dd-agent GID to int: %w", err)
	}
	return ddAgentUID, ddAgentGID, nil
}

// Ownership represents the desired ownership of a file.
type Ownership struct {
	Pattern string
	Owner   string
	Group   string
}

// Ownerships is a collection of ownerships.
type Ownerships []Ownership

// Ensure ensures that the file ownership is set to the desired state.
func (o Ownership) Ensure(rootPath string) error {
	uid, gid, err := getUserAndGroup(o.Owner, o.Group)
	if err != nil {
		return fmt.Errorf("error getting user and group IDs: %w", err)
	}
	matches, err := doublestar.Glob(os.DirFS(rootPath), o.Pattern, doublestar.WithFailOnIOErrors())
	if err != nil {
		return fmt.Errorf("error globbing pattern: %w", err)
	}
	for _, match := range matches {
		err = os.Chown(filepath.Join(rootPath, match), uid, gid)
		if err != nil {
			return fmt.Errorf("error changing file ownership: %w", err)
		}
	}
	return nil
}

// Ensure ensures that the file ownerships are set to the desired state.
func (os Ownerships) Ensure(rootPath string) error {
	for _, o := range os {
		if err := o.Ensure(rootPath); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSymlink ensures that the symlink is created.
func EnsureSymlink(source, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("error removing existing symlink: %w", err)
	}
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("error creating symlink: %w", err)
	}
	return nil
}
