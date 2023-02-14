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

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/probe/resolvers"
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
	ProcessResolver   *resolvers.ProcessResolver
	NamespaceResolver *resolvers.NamespaceResolver
	CgroupsResolver   *resolvers.CgroupsResolver
	TCResolver        *resolvers.TCResolver
	PathResolver      *resolvers.PathResolver
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

	pathResolver := resolvers.NewPathResolver(dentryResolver, mountResolver)

	containerResolver := &resolvers.ContainerResolver{}
	processResolver, err := resolvers.NewProcessResolver(probe.Manager, probe.Config, probe.StatsdClient,
		probe.scrubber, containerResolver, mountResolver, cgroupsResolver, userGroupResolver, timeResolver, pathResolver, resolvers.NewProcessResolverOpts(probe.Config.EnvsWithValue))
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		manager:           probe.Manager,
		MountResolver:     mountResolver,
		ContainerResolver: containerResolver,
		TimeResolver:      timeResolver,
		UserGroupResolver: userGroupResolver,
		TagsResolver:      resolvers.NewTagsResolver(config),
		DentryResolver:    dentryResolver,
		NamespaceResolver: namespaceResolver,
		CgroupsResolver:   cgroupsResolver,
		TCResolver:        tcResolver,
		ProcessResolver:   processResolver,
		PathResolver:      pathResolver,
	}

	return resolvers, nil
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

		if resolvers.IsKThread(uint32(ppid), uint32(proc.Pid)) {
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
