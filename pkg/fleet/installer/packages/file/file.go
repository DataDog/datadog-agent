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
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	userpkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

var userCache = sync.Map{}
var groupCache = sync.Map{}

// afterListingDirectory, when set, runs once a directory has been listed and before any of its
// entries is acted on. That is the only moment at which swapping a directory for a symlink
// could redirect a permission pass, so it is where tests stop this one to prove it cannot be.
// Nil outside tests.
var afterListingDirectory func(dir string)

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
	info, err := os.Stat(rootFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error stating root path: %w", err)
	}
	// Resolve symlinks on the root path only: it is trusted, and the package directory is
	// itself a symlink that a walk would refuse to recurse into. Everything below the root is
	// addressed through os.Root handles instead, so no component can be swapped for a symlink
	// to redirect these root-run operations outside the tree.
	rootFile, err = filepath.EvalSymlinks(rootFile)
	if err != nil {
		return fmt.Errorf("error resolving symlink: %w", err)
	}

	// Resolved once, before anything is opened: a cache miss forks getent, and those
	// milliseconds should not sit inside the walk.
	uid, gid := -1, -1
	if p.Owner != "" && p.Group != "" {
		uid, gid, err = getUserAndGroup(ctx, p.Owner, p.Group)
		if err != nil {
			return fmt.Errorf("error getting user and group IDs: %w", err)
		}
	}

	if !p.Recursive || !info.IsDir() {
		dir, name := filepath.Dir(rootFile), filepath.Base(rootFile)
		if name == "." || name == string(filepath.Separator) {
			dir, name = rootFile, "."
		}
		root, err := openRoot(dir)
		if root == nil {
			return err
		}
		defer root.Close()
		return p.apply(root, name, uid, gid)
	}

	root, err := openRoot(rootFile)
	if root == nil {
		return err
	}
	defer root.Close()
	return p.ensureTree(root, uid, gid)
}

// openRoot opens dir as an os.Root. A missing directory is reported as a nil root and a nil
// error, so callers skip it the way Ensure skips a missing path.
func openRoot(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("error opening directory: %w", err)
	}
	return root, nil
}

// ensureTree applies the permission to every entry below root, descending through nested
// os.Root handles so each operation names a single component relative to the directory that
// holds it. One handle per level of the tree is held at a time.
//
// Entries are handled before the directory itself: a mode without the search bit would
// otherwise stop the descent. Only real directories are descended into, so a symlink is an
// entry like any other and the reach of a recursive permission is unchanged.
func (p Permission) ensureTree(root *os.Root, uid, gid int) error {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error reading directory: %w", err)
	}
	if afterListingDirectory != nil {
		afterListingDirectory(root.Name())
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			if err := p.apply(root, entry.Name(), uid, gid); err != nil {
				return err
			}
			continue
		}
		child, err := root.OpenRoot(entry.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("error opening directory %s: %w", entry.Name(), err)
		}
		err = p.ensureTree(child, uid, gid)
		if cerr := child.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("error closing directory %s: %w", entry.Name(), cerr)
		}
		if err != nil {
			return err
		}
	}
	return p.apply(root, ".", uid, gid)
}

// apply sets the ownership and mode of one entry of root, named by a single path component.
func (p Permission) apply(root *os.Root, name string, uid, gid int) error {
	if uid >= 0 && gid >= 0 {
		// Lchown, not Chown: os.Root.Chown resolves an in-root symlink and acts on its target.
		if err := root.Lchown(name, uid, gid); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("error changing file ownership: %w", err)
		}
	}
	if p.Mode == 0 {
		return nil
	}
	info, err := root.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("error stating file: %w", err)
	}
	// Symlinks are skipped: there is no lchmod, and mode bits on a link are meaningless.
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	if err := root.Chmod(name, p.Mode); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error changing file mode: %w", err)
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

// UserAndGroupIDs returns the IDs of the given user and group. Lookups are cached for the
// lifetime of the process: resolving a name forks a getent subprocess, so callers that apply
// ownership themselves instead of going through Chown or Permission.Ensure should use this
// rather than the user package directly, or a hook ends up forking getent per entry.
func UserAndGroupIDs(ctx context.Context, username, group string) (uid, gid int, err error) {
	return getUserAndGroup(ctx, username, group)
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
		return fmt.Errorf("error getting user and group IDs for %s: %w", path, err)
	}
	// See Lchown: os.Chown already names the operation and the path.
	return os.Chown(path, uid, gid)
}
