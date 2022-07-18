// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	probe             *Probe
	DentryResolver    *DentryResolver
	MountResolver     *MountResolver
	ContainerResolver *ContainerResolver
	TimeResolver      *TimeResolver
	ProcessResolver   *ProcessResolver
	UserGroupResolver *UserGroupResolver
	TagsResolver      *TagsResolver
	NamespaceResolver *NamespaceResolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, probe *Probe) (*Resolvers, error) {
	dentryResolver, err := NewDentryResolver(probe)
	if err != nil {
		return nil, err
	}

	timeResolver, err := NewTimeResolver()
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := NewUserGroupResolver()
	if err != nil {
		return nil, err
	}

	mountResolver, err := NewMountResolver(probe)
	if err != nil {
		return nil, err
	}

	namespaceResolver, err := NewNamespaceResolver(probe)
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		probe:             probe,
		DentryResolver:    dentryResolver,
		MountResolver:     mountResolver,
		TimeResolver:      timeResolver,
		ContainerResolver: &ContainerResolver{},
		UserGroupResolver: userGroupResolver,
		TagsResolver:      NewTagsResolver(config),
		NamespaceResolver: namespaceResolver,
	}

	processResolver, err := NewProcessResolver(probe, resolvers, NewProcessResolverOpts(probe.config.CookieCacheSize))
	if err != nil {
		return nil, err
	}

	resolvers.ProcessResolver = processResolver

	return resolvers, nil
}

// resolveBasename resolves the inode to a filename
func (r *Resolvers) resolveBasename(e *model.FileFields) string {
	return r.DentryResolver.GetName(e.MountID, e.Inode, e.PathID)
}

// resolveFileFieldsPath resolves the inode to a full path
func (r *Resolvers) resolveFileFieldsPath(e *model.FileFields) (string, error) {
	pathStr, err := r.DentryResolver.Resolve(e.MountID, e.Inode, e.PathID, !e.HasHardLinks())
	if err != nil {
		return pathStr, err
	}

	_, mountPath, rootPath, mountErr := r.MountResolver.GetMountPath(e.MountID)
	if mountErr != nil {
		return pathStr, mountErr
	}

	if strings.HasPrefix(pathStr, rootPath) && rootPath != "/" {
		pathStr = strings.Replace(pathStr, rootPath, "", 1)
	}

	if mountPath != "/" {
		pathStr = mountPath + pathStr
	}

	return pathStr, err
}

// ResolveFileFieldsUser resolves the user id of the file to a username
func (r *Resolvers) ResolveFileFieldsUser(e *model.FileFields) string {
	if len(e.User) == 0 {
		e.User, _ = r.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveFileFieldsGroup resolves the group id of the file to a group name
func (r *Resolvers) ResolveFileFieldsGroup(e *model.FileFields) string {
	if len(e.Group) == 0 {
		e.Group, _ = r.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
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

// ResolvePCEContainerTags resolves the container tags of a ProcessCacheEntry
func (r *Resolvers) ResolvePCEContainerTags(e *model.ProcessCacheEntry) []string {
	if len(e.ContainerTags) == 0 && len(e.ContainerID) > 0 {
		e.ContainerTags = r.TagsResolver.Resolve(e.ContainerID)
	}
	return e.ContainerTags
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

	if err := r.DentryResolver.Start(r.probe); err != nil {
		return err
	}

	return r.NamespaceResolver.Start(ctx)
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	if err := r.snapshot(); err != nil {
		return fmt.Errorf("unable to snapshot processes: %w", err)
	}

	r.ProcessResolver.SetState(snapshotted)
	r.NamespaceResolver.SetState(snapshotted)

	selinuxStatusMap, err := r.probe.Map("selinux_enforce_status")
	if err != nil {
		return fmt.Errorf("unable to snapshot SELinux: %w", err)
	}

	if err := snapshotSELinux(selinuxStatusMap); err != nil {
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
		if err = r.MountResolver.SyncCache(proc); err != nil {
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
