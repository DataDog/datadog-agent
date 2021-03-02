// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/avast/retry-go"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/model"
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
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(probe *Probe, client *statsd.Client) (*Resolvers, error) {
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

	resolvers := &Resolvers{
		probe:             probe,
		DentryResolver:    dentryResolver,
		MountResolver:     NewMountResolver(probe),
		TimeResolver:      timeResolver,
		ContainerResolver: &ContainerResolver{},
		UserGroupResolver: userGroupResolver,
	}

	processResolver, err := NewProcessResolver(probe, resolvers, client, NewProcessResolverOpts(true, probe.config.CookieCacheSize))
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

// resolveContainerPath resolves the inode to a path relative to the container
func (r *Resolvers) resolveContainerPath(e *model.FileFields) string {
	containerPath, _, _, err := r.MountResolver.GetMountPath(e.MountID)
	if err != nil {
		return ""
	}
	return containerPath
}

// resolveInode resolves the inode to a full path. Returns the path and true if it was entirely resolved
func (r *Resolvers) resolveInode(e *model.FileFields) (string, error) {
	pathStr, err := r.DentryResolver.Resolve(e.MountID, e.Inode, e.PathID)
	if pathStr == dentryPathKeyNotFound || err != nil {
		return pathStr, err
	}

	_, mountPath, rootPath, err := r.MountResolver.GetMountPath(e.MountID)
	if err == nil {
		if strings.HasPrefix(pathStr, rootPath) && rootPath != "/" {
			pathStr = strings.Replace(pathStr, rootPath, "", 1)
		}
		pathStr = path.Join(mountPath, pathStr)
	}

	return pathStr, err
}

// ResolveInode resolves the inode to a full path. Returns the path and true if it was entirely resolved
func (r *Resolvers) ResolveInode(e *model.FileEvent) string {
	path, _ := r.resolveInode(&e.FileFields)
	return path
}

// ResolveUser resolves the user id of the file to a username
func (r *Resolvers) ResolveUser(e *model.FileFields) string {
	if len(e.User) == 0 {
		e.User, _ = r.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveGroup resolves the group id of the file to a group name
func (r *Resolvers) ResolveGroup(e *model.FileFields) string {
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

// ResolveProcessUser resolves the user id of the process to a username
func (r *Resolvers) ResolveProcessUser(p *model.ProcessContext) string {
	if len(p.User) == 0 {
		p.User, _ = r.UserGroupResolver.ResolveUser(int(p.UID))
	}
	return p.User
}

// ResolveProcessGroup resolves the group id of the process to a group name
func (r *Resolvers) ResolveProcessGroup(p *model.ProcessContext) string {
	if len(p.Group) == 0 {
		p.Group, _ = r.UserGroupResolver.ResolveGroup(int(p.GID))
	}
	return p.Group
}

// Start the resolvers
func (r *Resolvers) Start(ctx context.Context) error {
	if err := r.ProcessResolver.Start(ctx); err != nil {
		return err
	}

	return r.DentryResolver.Start()
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	if err := retry.Do(r.snapshot, retry.Delay(0), retry.Attempts(5)); err != nil {
		return errors.Wrap(err, "unable to snapshot processes")
	}

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

	cacheModified := false

	for _, proc := range processes {
		// Start with the mount resolver because the process resolver might need it to resolve paths
		if err := r.MountResolver.SyncCache(proc); err != nil {
			if !os.IsNotExist(err) {
				log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't sync mount points", proc.Pid))
			}
		}

		// Sync the process cache
		cacheModified = r.ProcessResolver.SyncCache(proc)
	}

	// There is a possible race condition when a process starts right after we called process.AllProcesses
	// and before we inserted the cache entry of its parent. Call Snapshot again until we do not modify the
	// process cache anymore
	if cacheModified {
		return errors.New("cache modified")
	}

	return nil
}
