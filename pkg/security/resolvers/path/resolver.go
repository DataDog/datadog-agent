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
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

// ResolveBasename resolves an inode/mount ID pair to a file basename
func (r *Resolver) ResolveBasename(e *model.FileFields) string {
	return r.dentryResolver.ResolveName(e.PathKey)
}

// ResolveFilePath resolves an inode/mount ID pair to a full path
func (r *Resolver) ResolveFilePath(e *model.FileFields, _ *model.PIDContext, _ *model.ContainerContext) (string, error) {
	pathStr, err := r.dentryResolver.Resolve(e.PathKey, !e.HasHardLinks())
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: err}
		}
		return pathStr, &ErrPathResolution{Err: err}
	}

	return pathStr, nil
}

// ResolveFileFieldsPath resolves an inode/mount ID pair to a full path along with its mount path
func (r *Resolver) ResolveFileFieldsPath(e *model.FileFields, pidCtx *model.PIDContext, ctrCtx *model.ContainerContext) (string, string, model.MountSource, model.MountOrigin, error) {
	pathStr, err := r.ResolveFilePath(e, pidCtx, ctrCtx)
	if err != nil {
		return pathStr, "", model.MountSourceUnknown, model.MountOriginUnknown, err
	}

	if e.IsFileless() {
		return pathStr, "", model.MountSourceMountID, model.MountOriginEvent, nil
	}

	mountPath, source, origin, err := r.mountResolver.ResolveMountPath(e.MountID, e.Device, pidCtx.Pid, ctrCtx.ContainerID)
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, "", origin, source, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, "", source, origin, &ErrPathResolution{Err: err}
	}

	rootPath, source, origin, err := r.mountResolver.ResolveMountRoot(e.MountID, e.Device, pidCtx.Pid, ctrCtx.ContainerID)
	if err != nil {
		if _, err := r.mountResolver.IsMountIDValid(e.MountID); errors.Is(err, mount.ErrMountKernelID) {
			return pathStr, "", source, origin, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, "", source, origin, &ErrPathResolution{Err: err}
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

func resolveFromProc(mountid int, pce *model.ProcessCacheEntry) string {
	if pce == nil {
		return ""
	}

	pids, _ := pce.GetContainerPIDs()
	pids = append(pids, pce.Pid)

	for _, pid := range pids {
		mounts, err := kernel.ParseMountInfoFile(int32(pid))
		if err != nil {
			break
		}

		for _, mnt := range mounts {
			if mnt.ID == mountid {
				return mnt.Mountpoint
			}
		}
	}

	return ""
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

// SetMountRoot set the mount point information
func (r *Resolver) SetMountRoot(ev *model.Event, e *model.Mount) error {
	var err error
	e.RootStr, err, e.RootStrSrc = r.dentryResolver.ResolveSrc(e.RootPathKey, true)

	mountPointFromProc := resolveFromProc(int(e.RootPathKey.MountID), ev.ProcessCacheEntry)

	if mountPointFromProc != "" && mountPointFromProc != e.MountPointStr {
		seclog.Errorf("Different mountpoint detected: From proc: %s. e.MountPointStr=%s. mountEvent=%+v ", mountPointFromProc, e.MountPointStr, e)
		//fmt.Printf("MNTP SetMountRoot :: (Different mountpoint). MountpointFromProc = %s. e.MountPointStr=%s. MountEvent=%+v\n", mountPointFromProc, e.MountPointStr, e)
	}

	if err != nil {
		return &ErrPathResolutionNotCritical{Err: err}
	}
	return nil
}

// SetMountPoint set the mount point information
func (r *Resolver) SetMountPoint(ev *model.Event, e *model.Mount) error {
	var err error

	e.MountPointStr, err, e.MountPointStrSrc = r.dentryResolver.ResolveSrc(e.ParentPathKey, true)

	mountPointFromProc := resolveFromProc(int(e.MountID), ev.ProcessCacheEntry)

	if mountPointFromProc != "" && mountPointFromProc != e.MountPointStr {
		seclog.Errorf("Different mountpoint detected: From proc: %s. e.MountPointStr=%s | mountEvent =%+v", mountPointFromProc, e.MountPointStr, e)
		//fmt.Printf("MNTP SetMountPoint :: (Different mountpoint). From proc: %s. e.MountPointStr=%s. MountEvent=%+v\n", mountPointFromProc, e.MountPointStr, e)
	}

	if err != nil {
		return &ErrPathResolutionNotCritical{Err: err}
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
