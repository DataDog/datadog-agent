// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package file offers filesystem utils geared towards idempotent operations.
package file

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
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

// Permission represents the desired ownership and mode of a file.
type Permission struct {
	Path      string
	Owner     string
	Group     string
	Mode      os.FileMode
	Recursive bool
}

// Permissions is a collection of Permission.
type Permissions []Permission

// Ensure ensures that the file ownership and mode are set to the desired state.
func (p Permission) Ensure(rootPath string) error {
	rootFile := filepath.Join(rootPath, p.Path)
	_, err := os.Stat(rootFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error stating root path: %w", err)
	}
	// Resolve symlinks to ensure we're changing the permissions of the actual file and avoid issues with `filepath.Walk`.
	rootFile, err = filepath.EvalSymlinks(rootFile)
	if err != nil {
		return fmt.Errorf("error resolving symlink: %w", err)
	}
	files := []string{rootFile}
	if p.Recursive {
		files, err = filesInDir(rootFile)
		if err != nil {
			return fmt.Errorf("error getting files in directory: %w", err)
		}
	}
	for _, file := range files {
		if p.Owner != "" && p.Group != "" {
			if err := chown(file, p.Owner, p.Group); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("error changing file ownership: %w", err)
			}
		}
		if p.Mode != 0 {
			if err := os.Chmod(file, p.Mode); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("error changing file mode: %w", err)
			}
		}
	}
	return nil
}

// Ensure ensures that the file ownership and mode are set to the desired state.
func (ps Permissions) Ensure(rootPath string) error {
	for _, o := range ps {
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

func getUserAndGroup(username, group string) (uid, gid int, err error) {
	rawUID, err := user.Lookup(username)
	if err != nil {
		return 0, 0, fmt.Errorf("error looking up user: %w", err)
	}
	rawGID, err := user.LookupGroup(group)
	if err != nil {
		return 0, 0, fmt.Errorf("error looking up group: %w", err)
	}
	uid, err = strconv.Atoi(rawUID.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("error converting UID to int: %w", err)
	}
	gid, err = strconv.Atoi(rawGID.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("error converting GID to int: %w", err)
	}
	return uid, gid, nil
}

func chown(path string, username string, group string) (err error) {
	uid, gid, err := getUserAndGroup(username, group)
	if err != nil {
		return fmt.Errorf("error getting user and group IDs: %w", err)
	}
	err = os.Chown(path, uid, gid)
	if err != nil {
		return fmt.Errorf("error changing file ownership: %w", err)
	}
	return nil
}

func filesInDir(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("error walking path: %w", err)
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
