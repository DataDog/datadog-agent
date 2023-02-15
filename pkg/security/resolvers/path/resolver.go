// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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
	mountResolver  *mount.Resolver
}

// NewResolver returns a new path resolver
func NewResolver(dentryResolver *dentry.Resolver, mountResolver *mount.Resolver) *Resolver {
	return &Resolver{dentryResolver: dentryResolver, mountResolver: mountResolver}
}

// ResolveBasename resolves an inode/mount ID pair to a file basename
func (r *Resolver) ResolveBasename(e *model.FileFields) string {
	return r.dentryResolver.ResolveName(e.MountID, e.Inode, e.PathID)
}

// ResolveFileFieldsPath resolves an inode/mount ID pair to a full path
func (r *Resolver) ResolveFileFieldsPath(e *model.FileFields, pidCtx *model.PIDContext, ctrCtx *model.ContainerContext) (string, error) {
	pathStr, err := r.dentryResolver.Resolve(e.MountID, e.Inode, e.PathID, !e.HasHardLinks())
	if err != nil {
		return pathStr, err
	}

	if e.IsFileless() {
		return pathStr, err
	}

	mountPath, err := r.mountResolver.ResolveMountPath(e.MountID, pidCtx.Pid, ctrCtx.ID)
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, err
	}

	rootPath, err := r.mountResolver.ResolveMountRoot(e.MountID, pidCtx.Pid, ctrCtx.ID)
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, err
	}
	// This aims to handle bind mounts
	if strings.HasPrefix(pathStr, rootPath) && rootPath != "/" {
		pathStr = strings.Replace(pathStr, rootPath, "", 1)
	}

	if mountPath != "/" {
		pathStr = mountPath + pathStr
	}

	return pathStr, err
}

// SetMountRoot set the mount point information
func (r *Resolver) SetMountRoot(ev *model.Event, e *model.Mount) error {
	var err error
	e.RootStr, err = r.dentryResolver.Resolve(e.RootMountID, e.RootInode, 0, true)
	return err
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
func (r *Resolver) SetMountPoint(ev *model.Event, e *model.Mount) error {
	var err error
	e.MountPointStr, err = r.dentryResolver.Resolve(e.ParentMountID, e.ParentInode, 0, true)
	return err
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
