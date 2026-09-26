// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
)

// ensurePermissionsInDir applies the ownership and mode of each named permission to an entry
// of dir. It is the symlink-safe counterpart of file.Permissions.Ensure for configuration
// directories that the unprivileged dd-agent user can write to: file.Permissions.Ensure
// resolves symlinks on purpose, which would let a planted symlink hand a root-owned file to
// dd-agent. Missing entries are skipped, as they are by file.Permissions.Ensure.
// nolint:unused // Called only from platform-specific code/contexts
func ensurePermissionsInDir(ctx HookContext, dir string, permissions file.Permissions) (err error) {
	span, _ := ctx.StartSpan("ensure_permissions_in_dir")
	defer func() { span.Finish(err) }()
	span.SetTag("dir", dir)

	root, err := os.OpenRoot(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("could not open %s: %w", dir, err)
	}
	defer root.Close()

	for _, permission := range permissions {
		if permission.Recursive {
			return fmt.Errorf("recursive permission %q is not supported", permission.Path)
		}
		// Resolved before the entry is inspected: a cache miss forks getent, and those
		// milliseconds must not sit between the check below and the operations that follow it.
		var uid, gid int
		if permission.Owner != "" && permission.Group != "" {
			var err error
			uid, gid, err = file.UserAndGroupIDs(ctx, permission.Owner, permission.Group)
			if err != nil {
				return fmt.Errorf("could not resolve %s:%s: %w", permission.Owner, permission.Group, err)
			}
		}
		info, err := root.Lstat(permission.Path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not stat %s in %s: %w", permission.Path, dir, err)
		}
		// Fail loudly rather than re-permissioning whatever the link points at. These are
		// package-managed files, so a symlink here means the directory was tampered with.
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to set permissions on %s in %s: it is a symlink", permission.Path, dir)
		}
		if permission.Owner != "" && permission.Group != "" {
			// Lchown, not Chown: os.Root.Chown resolves an in-root symlink and acts on its
			// target, so an entry swapped for a link after the Lstat above would redirect this
			// root-run chown onto another root-owned file in the directory. For a regular file
			// the two are identical.
			if err := root.Lchown(permission.Path, uid, gid); err != nil {
				return fmt.Errorf("could not change ownership of %s in %s: %w", permission.Path, dir, err)
			}
		}
		if permission.Mode != 0 {
			// There is no lchmod, so this one keeps the same shape. os.Root still bounds the
			// blast radius: a lost race can only reach a file inside dir, never outside it.
			if err := root.Chmod(permission.Path, permission.Mode); err != nil {
				return fmt.Errorf("could not change mode of %s in %s: %w", permission.Path, dir, err)
			}
		}
	}
	return nil
}
