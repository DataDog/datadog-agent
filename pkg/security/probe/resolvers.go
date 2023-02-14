// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/probe/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	manager           *manager.Manager
	MountResolver     *resolvers.MountResolver
	ContainerResolver *resolvers.ContainerResolver
	TimeResolver      *resolvers.TimeResolver
	UserGroupResolver *resolvers.UserGroupResolver
	TagsResolver      *resolvers.TagsResolver
	DentryResolver    *resolvers.DentryResolver
	ProcessResolver   *ProcessResolver
	NamespaceResolver *resolvers.NamespaceResolver
	CgroupsResolver   *resolvers.CgroupsResolver
	TCResolver        *resolvers.TCResolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, probe *Probe) (*Resolvers, error) {
	dentryResolver, err := resolvers.NewDentryResolver(probe.Config, probe.StatsdClient, probe.Erpc)
	if err != nil {
		return nil, err
	}

	timeResolver, err := resolvers.NewTimeResolver()
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := resolvers.NewUserGroupResolver()
	if err != nil {
		return nil, err
	}

	tcResolver := resolvers.NewTCResolver(config)

	namespaceResolver, err := resolvers.NewNamespaceResolver(probe.Config, probe.Manager, probe.StatsdClient, probe.resolvers.TCResolver)
	if err != nil {
		return nil, err
	}

	cgroupsResolver, err := resolvers.NewCgroupsResolver()
	if err != nil {
		return nil, err
	}

	mountResolver, err := resolvers.NewMountResolver(probe.StatsdClient, cgroupsResolver, resolvers.MountResolverOpts{UseProcFS: true})
	if err != nil {
		return nil, err
	}

	processResolver, err := NewProcessResolver(probe.Manager, probe.Config, probe.StatsdClient,
		probe.scrubber, NewProcessResolverOpts(probe.Config.EnvsWithValue))
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		manager:           probe.Manager,
		MountResolver:     mountResolver,
		ContainerResolver: &resolvers.ContainerResolver{},
		TimeResolver:      timeResolver,
		UserGroupResolver: userGroupResolver,
		TagsResolver:      resolvers.NewTagsResolver(config),
		DentryResolver:    dentryResolver,
		NamespaceResolver: namespaceResolver,
		CgroupsResolver:   cgroupsResolver,
		TCResolver:        tcResolver,
		ProcessResolver:   processResolver,
	}

	resolvers.ProcessResolver.resolvers = resolvers

	return resolvers, nil
}

// resolveBasename resolves an inode/mount ID pair to a file basename
func (r *Resolvers) resolveBasename(e *model.FileFields) string {
	return r.DentryResolver.ResolveName(e.MountID, e.Inode, e.PathID)
}

// resolveFileFieldsPath resolves an inode/mount ID pair to a full path
func (r *Resolvers) resolveFileFieldsPath(e *model.FileFields, pidCtx *model.PIDContext, ctrCtx *model.ContainerContext) (string, error) {
	pathStr, err := r.DentryResolver.Resolve(e.MountID, e.Inode, e.PathID, !e.HasHardLinks())
	if err != nil {
		return pathStr, err
	}

	if e.IsFileless() {
		return pathStr, err
	}

	mountPath, err := r.MountResolver.ResolveMountPath(e.MountID, pidCtx.Pid, ctrCtx.ID)
	if err != nil {
		if _, err := r.MountResolver.IsMountIDValid(e.MountID); errors.Is(err, resolvers.ErrMountKernelID) {
			return pathStr, &ErrPathResolutionNotCritical{Err: fmt.Errorf("mount ID(%d) invalid: %w", e.MountID, err)}
		}
		return pathStr, err
	}

	rootPath, err := r.MountResolver.ResolveMountRoot(e.MountID, pidCtx.Pid, ctrCtx.ID)
	if err != nil {
		if _, err := r.MountResolver.IsMountIDValid(e.MountID); errors.Is(err, resolvers.ErrMountKernelID) {
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

// ResolveCredentialsUser resolves the user id of the process to a username
func (r *Resolvers) ResolveCredentialsUser(e *model.Credentials) string {
	if len(e.User) == 0 {
		e.User, _ = r.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveCredentialsGroup resolves the group id of the process to a group name
func (r *Resolvers) ResolveCredentialsGroup(e *model.Credentials) string {
	if len(e.Group) == 0 {
		e.Group, _ = r.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
}

// ResolveCredentialsEUser resolves the effective user id of the process to a username
func (r *Resolvers) ResolveCredentialsEUser(e *model.Credentials) string {
	if len(e.EUser) == 0 {
		e.EUser, _ = r.UserGroupResolver.ResolveUser(int(e.EUID))
	}
	return e.EUser
}

// ResolveCredentialsEGroup resolves the effective group id of the process to a group name
func (r *Resolvers) ResolveCredentialsEGroup(e *model.Credentials) string {
	if len(e.EGroup) == 0 {
		e.EGroup, _ = r.UserGroupResolver.ResolveGroup(int(e.EGID))
	}
	return e.EGroup
}

// ResolveCredentialsFSUser resolves the file-system user id of the process to a username
func (r *Resolvers) ResolveCredentialsFSUser(e *model.Credentials) string {
	if len(e.FSUser) == 0 {
		e.FSUser, _ = r.UserGroupResolver.ResolveUser(int(e.FSUID))
	}
	return e.FSUser
}

// ResolveCredentialsFSGroup resolves the file-system group id of the process to a group name
func (r *Resolvers) ResolveCredentialsFSGroup(e *model.Credentials) string {
	if len(e.FSGroup) == 0 {
		e.FSGroup, _ = r.UserGroupResolver.ResolveGroup(int(e.FSGID))
	}
	return e.FSGroup
}

// Start the resolvers
func (r *Resolvers) Start(ctx context.Context) error {
	if err := r.ProcessResolver.Start(ctx); err != nil {
		return err
	}
	r.MountResolver.Start(ctx)

	if err := r.TagsResolver.Start(ctx); err != nil {
		return err
	}

	if err := r.DentryResolver.Start(r.manager); err != nil {
		return err
	}

	return r.NamespaceResolver.Start(ctx)
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	if err := r.snapshot(); err != nil {
		return fmt.Errorf("unable to snapshot processes: %w", err)
	}

	r.ProcessResolver.SetState(resolvers.Snapshotted)
	r.NamespaceResolver.SetState(resolvers.Snapshotted)

	selinuxStatusMap, err := managerhelper.Map(r.manager, "selinux_enforce_status")
	if err != nil {
		return fmt.Errorf("unable to snapshot SELinux: %w", err)
	}

	if err := resolvers.SnapshotSELinux(selinuxStatusMap); err != nil {
		return err
	}

	runtime.GC()
	return nil
}

// snapshot internal version of Snapshot. Calls the relevant resolvers to sync their caches.
func (r *Resolvers) snapshot() error {
	// List all processes, to trigger the process and mount snapshots
	processes, err := utils.GetProcesses()
	if err != nil {
		return err
	}

	// make sure to insert them in the creation time order
	sort.Slice(processes, func(i, j int) bool {
		procA := processes[i]
		procB := processes[j]

		createA, err := procA.CreateTime()
		if err != nil {
			return processes[i].Pid < processes[j].Pid
		}

		createB, err := procB.CreateTime()
		if err != nil {
			return processes[i].Pid < processes[j].Pid
		}

		if createA == createB {
			return processes[i].Pid < processes[j].Pid
		}

		return createA < createB
	})

	for _, proc := range processes {
		ppid, err := proc.Ppid()
		if err != nil {
			continue
		}

		if IsKThread(uint32(ppid), uint32(proc.Pid)) {
			continue
		}

		// Start with the mount resolver because the process resolver might need it to resolve paths
		if err = r.MountResolver.SyncCache(uint32(proc.Pid)); err != nil {
			if !os.IsNotExist(err) {
				log.Debugf("snapshot failed for %d: couldn't sync mount points: %s", proc.Pid, err)
			}
		}

		// Sync the process cache
		r.ProcessResolver.SyncCache(proc)

		// Sync the namespace cache
		r.NamespaceResolver.SyncCache(proc)
	}

	return nil
}

// Close cleans up any underlying resolver that requires a cleanup
func (r *Resolvers) Close() error {
	// clean up the dentry resolver eRPC segment
	return r.DentryResolver.Close()
}
