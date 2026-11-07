// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package path holds path related files
package path

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Resolver describes a resolvers for path and file names
type Resolver struct {
	dentryResolver *dentry.Resolver
	mountResolver  mount.ResolverInterface
}

// NewResolver returns a new path resolver
func NewResolver(dentryResolver *dentry.Resolver, mountResolver mount.ResolverInterface) *Resolver {
	return &Resolver{dentryResolver: dentryResolver, mountResolver: mountResolver}
}

// fileCaller returns the dentry resolution caller of a file
func fileCaller(e *model.FileFields) dentry.Caller {
	if e.HasHardLinks() {
		return dentry.CallerFileHardLink
	}
	return dentry.CallerFile
}

// ResolveBasename resolves an inode/mount ID pair to a file basename
func (r *Resolver) ResolveBasename(e *model.FileFields) string {
	return r.dentryResolver.ResolveName(e.PathKey, true, fileCaller(e))
}

// ResolveDentryFilePath resolves an inode/mount ID pair to a path
func (r *Resolver) ResolveDentryFilePath(e *model.FileFields) (string, error) {
	pathStr, err := r.dentryResolver.Resolve(e.PathKey, true, fileCaller(e))
	if err != nil {
		if _, mountErr := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(mountErr, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: fmt.Errorf("dentry resolution failed (mount_id=%d invalid: %w): %w", e.MountID, mountErr, err)}
		}
		return pathStr, &ErrPathResolution{Err: fmt.Errorf("dentry resolution failed: %w", err)}
	}

	return pathStr, nil
}

// ResolveMountAttributes resolves the mount attributes of the mountpoint of a file
func (r *Resolver) ResolveMountAttributes(e *model.FileEvent, pidCtx *model.PIDContext) error {
	if e.MountVisibilityResolved || e.IsFileless() {
		return nil
	}

	mnt, _, _, err := r.mountResolver.ResolveMount(e.MountID, pidCtx.Pid)
	if err != nil {
		return fmt.Errorf("mount attribute resolution failed (mount_id=%d, pid=%d): %w", e.MountID, pidCtx.Pid, err)
	}

	e.MountVisibilityResolved = true
	e.MountVisible = mnt.Visible
	e.MountDetached = mnt.Detached
	e.Filesystem = mnt.FSType

	return nil
}

// ResolveFullFilePath resolves an inode/mount ID pair to a full path along with its mount path
func (r *Resolver) ResolveFullFilePath(e *model.FileFields, pidCtx *model.PIDContext) (string, string, model.MountSource, model.MountOrigin, error) {
	pathStr, err := r.ResolveDentryFilePath(e)
	if err != nil {
		return pathStr, "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	if e.IsFileless() {
		return pathStr, "", model.MountSourceMountID, model.MountOriginEvent, nil
	}

	mountPath, source, origin, err := r.mountResolver.ResolveMountPath(e.MountID, pidCtx.Pid)
	if err != nil {
		if _, mountErr := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(mountErr, mount.ErrMountKernelID) {
			return pathStr, "", origin, source, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount path resolution failed for dentry %q (mount_id=%d, pid=%d, mount_check=%w): %w", pathStr, e.MountID, pidCtx.Pid, mountErr, err)}
		}
		return pathStr, "", source, origin, &ErrPathResolution{Err: fmt.Errorf("mount path resolution failed for dentry %q (mount_id=%d, pid=%d): %w", pathStr, e.MountID, pidCtx.Pid, err)}
	}

	rootPath, source, origin, err := r.mountResolver.ResolveMountRoot(e.MountID, pidCtx.Pid)
	if err != nil {
		if _, mountErr := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(mountErr, mount.ErrMountKernelID) {
			return pathStr, "", source, origin, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount root resolution failed for dentry %q on mount %q (mount_id=%d, pid=%d, mount_check=%w): %w", pathStr, mountPath, e.MountID, pidCtx.Pid, mountErr, err)}
		}
		return pathStr, "", source, origin, &ErrPathResolution{Err: fmt.Errorf("mount root resolution failed for dentry %q on mount %q (mount_id=%d, pid=%d): %w", pathStr, mountPath, e.MountID, pidCtx.Pid, err)}
	}

	// This aims to handle bind mounts
	if rootPath != "/" {
		pathStr = strings.TrimPrefix(pathStr, rootPath)
	}

	if mountPath != "/" {
		if pathStr != "/" {
			pathStr = mountPath + pathStr
		} else {
			pathStr = mountPath
		}
	}

	return pathStr, mountPath, source, origin, nil
}

// SetMountRoot set the mount point information
func (r *Resolver) SetMountRoot(_ *model.Event, e *model.Mount) error {
	var err error

	e.RootStr, err = r.dentryResolver.Resolve(e.RootPathKey, true, dentry.CallerMountRoot)
	if err != nil {
		return &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount root dentry resolution failed (mount_id=%d): %w", e.MountID, err)}
	}
	return nil
}

// ResolveMountRoot resolves the mountpoint to a full path
func (r *Resolver) ResolveMountRoot(ev *model.Event, e *model.Mount) (string, error) {
	if len(e.RootStr) == 0 {
		if err := r.SetMountRoot(ev, e); err != nil {
			return "", err
		}
	}
	return e.RootStr, nil
}

// SetMountPoint set the mount point information
func (r *Resolver) SetMountPoint(_ *model.Event, e *model.Mount) error {
	var err error

	e.MountPointStr, err = r.dentryResolver.Resolve(e.ParentPathKey, true, dentry.CallerMountPoint)
	if err != nil {
		return &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount point dentry resolution failed (mount_id=%d, parent_mount_id=%d): %w", e.MountID, e.ParentPathKey.MountID, err)}
	}
	return nil
}

// ResolveMountPoint resolves the mountpoint to a full path
func (r *Resolver) ResolveMountPoint(ev *model.Event, e *model.Mount) (string, error) {
	if len(e.MountPointStr) == 0 {
		if err := r.SetMountPoint(ev, e); err != nil {
			return "", err
		}
	}
	return e.MountPointStr, nil
}
