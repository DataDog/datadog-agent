// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package file offers filesystem utils geared towards idempotent operations.
package file

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	userpkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

var userCache = sync.Map{}
var groupCache = sync.Map{}

// Path is a path to a file or directory.
type Path string

// EnsureAbsent ensures that the path does not exist and removes it if it does.
func (p Path) EnsureAbsent(ctx context.Context, rootPath string) error {
	span, _ := telemetry.StartSpanFromContext(ctx, "ensure_path_absent")
	defer func() {
		span.Finish(nil)
	}()
	span.SetTag("path", filepath.Join(rootPath, string(p)))
	matches, err := filepath.Glob(filepath.Join(rootPath, string(p)))
	if err != nil {
		return fmt.Errorf("error globbing path: %w", err)
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			return fmt.Errorf("error removing path: %w", err)
		}
	}
	return nil
}

// Paths is a collection of Path.
type Paths []Path

// EnsureAbsent ensures that the paths do not exist and removes them if they do.
func (ps Paths) EnsureAbsent(ctx context.Context, rootPath string) error {
	for _, p := range ps {
		if err := p.EnsureAbsent(ctx, rootPath); err != nil {
			return err
		}
	}
	return nil
}

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
func (d Directory) Ensure(ctx context.Context) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "ensure_directory")
	defer func() {
		span.Finish(err)
	}()
	span.SetTag("path", d.Path)
	span.SetTag("owner", d.Owner)
	span.SetTag("group", d.Group)
	span.SetTag("mode", d.Mode)

	uid, gid, err := getUserAndGroup(ctx, d.Owner, d.Group)
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
func (ds Directories) Ensure(ctx context.Context) error {
	for _, d := range ds {
		if err := d.Ensure(ctx); err != nil {
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
func (p Permission) Ensure(ctx context.Context, rootPath string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "ensure_permission")
	defer func() {
		span.Finish(err)
	}()
	span.SetTag("path", rootPath)
	span.SetTag("owner", p.Owner)
	span.SetTag("group", p.Group)
	span.SetTag("mode", p.Mode)
	span.SetTag("recursive", p.Recursive)

	rootFile := filepath.Join(rootPath, p.Path)
	_, err = os.Stat(rootFile)
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
			if err := Chown(ctx, file, p.Owner, p.Group); err != nil && !errors.Is(err, os.ErrNotExist) {
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
func (ps Permissions) Ensure(ctx context.Context, rootPath string) error {
	for _, o := range ps {
		if err := o.Ensure(ctx, rootPath); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSymlink ensures that the symlink is created.
func EnsureSymlink(ctx context.Context, source, target string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "ensure_symlink")
	defer func() {
		span.Finish(err)
	}()
	span.SetTag("source", source)
	span.SetTag("target", target)
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("error removing existing symlink: %w", err)
	}
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("error creating symlink: %w", err)
	}
	return nil
}

// EnsureSymlinkAbsent ensures that the symlink is removed.
func EnsureSymlinkAbsent(ctx context.Context, target string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "ensure_symlink")
	defer func() {
		span.Finish(err)
	}()
	span.SetTag("target", target)
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("error removing existing symlink: %w", err)
	}
	return nil
}

func getUserAndGroup(ctx context.Context, username, group string) (uid, gid int, err error) {
	// Use internal user package GetUserID and GetGroupID, caching as before for efficiency
	uidRaw, uidOk := userCache.Load(username)
	if !uidOk {
		uidRaw, err = userpkg.GetUserID(ctx, username)
		if err != nil {
			return 0, 0, fmt.Errorf("error getting user ID for %s: %w", username, err)
		}
		userCache.Store(username, uidRaw)
	}

	gidRaw, gidOk := groupCache.Load(group)
	if !gidOk {
		gidRaw, err = userpkg.GetGroupID(ctx, group)
		if err != nil {
			return 0, 0, fmt.Errorf("error getting group ID for %s: %w", group, err)
		}
		groupCache.Store(group, gidRaw)
	}

	uid, ok := uidRaw.(int)
	if !ok {
		return 0, 0, fmt.Errorf("error converting UID to int: %v", uidRaw)
	}
	gid, ok = gidRaw.(int)
	if !ok {
		return 0, 0, fmt.Errorf("error converting GID to int: %v", gidRaw)
	}

	return uid, gid, nil
}

// Chown changes the ownership of a file to the specified owner and group.
func Chown(ctx context.Context, path string, username string, group string) (err error) {
	uid, gid, err := getUserAndGroup(ctx, username, group)
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
	err := filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
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
