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

	"github.com/avast/retry-go"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
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
	TagsResolver      *TagsResolver
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

	resolvers := &Resolvers{
		probe:             probe,
		DentryResolver:    dentryResolver,
		MountResolver:     NewMountResolver(probe),
		TimeResolver:      timeResolver,
		ContainerResolver: &ContainerResolver{},
		UserGroupResolver: userGroupResolver,
		TagsResolver:      NewTagsResolver(config),
	}

	processResolver, err := NewProcessResolver(probe, resolvers, probe.statsdClient, NewProcessResolverOpts(probe.config.CookieCacheSize))
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

// resolveFileFieldsPath resolves the inode to a full path. Returns the path and true if it was entirely resolved
func (r *Resolvers) resolveFileFieldsPath(e *model.FileFields) (string, error) {
	pathStr, err := r.DentryResolver.Resolve(e.MountID, e.Inode, e.PathID)
	if pathStr == dentryPathKeyNotFound {
		return pathStr, err
	}

	_, mountPath, rootPath, mountErr := r.MountResolver.GetMountPath(e.MountID)
	if mountErr != nil {
		return "", mountErr
	}

	if strings.HasPrefix(pathStr, rootPath) && rootPath != "/" {
		pathStr = strings.Replace(pathStr, rootPath, "", 1)
	}
	pathStr = path.Join(mountPath, pathStr)

	return pathStr, err
}

// ResolveFilePath resolves the inode to a full path. Returns the path and true if it was entirely resolved
func (r *Resolvers) ResolveFilePath(e *model.FileEvent) string {
	path, _ := r.resolveFileFieldsPath(&e.FileFields)
	return path
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

// Start the resolvers
func (r *Resolvers) Start(ctx context.Context) error {
	if err := r.ProcessResolver.Start(ctx); err != nil {
		return err
	}
	r.MountResolver.Start(ctx)

	if err := r.TagsResolver.Start(ctx); err != nil {
		return err
	}

	return r.DentryResolver.Start(r.probe)
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	if err := retry.Do(r.snapshot, retry.Delay(0), retry.Attempts(5)); err != nil {
		return errors.Wrap(err, "unable to snapshot processes")
	}

	r.ProcessResolver.SetState(snapshotted)

	selinuxStatusMap, err := r.probe.Map("selinux_enforce_status")
	if err != nil {
		return errors.Wrap(err, "unable to snapshot SELinux")
	}
	return snapshotSELinux(selinuxStatusMap)
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

// Close cleans up any underlying resolver that requires a cleanup
func (r *Resolvers) Close() error {
	// clean up the dentry resolver eRPC segment
	return r.DentryResolver.Close()
}
