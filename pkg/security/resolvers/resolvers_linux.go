// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package resolvers

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/container"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/selinux"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	manager           *manager.Manager
	MountResolver     *mount.Resolver
	ContainerResolver *container.Resolver
	TimeResolver      *time.Resolver
	UserGroupResolver *usergroup.Resolver
	TagsResolver      *tags.Resolver
	DentryResolver    *dentry.Resolver
	ProcessResolver   *process.Resolver
	NamespaceResolver *netns.Resolver
	CGroupResolver    *cgroup.Resolver
	TCResolver        *tc.Resolver
	PathResolver      *path.Resolver
	SBOMResolver      *sbom.Resolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, manager *manager.Manager, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber, eRPC *erpc.ERPC) (*Resolvers, error) {
	dentryResolver, err := dentry.NewResolver(config.Probe, statsdClient, eRPC)
	if err != nil {
		return nil, err
	}

	timeResolver, err := time.NewResolver()
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := usergroup.NewResolver()
	if err != nil {
		return nil, err
	}

	tcResolver := tc.NewResolver(config.Probe)

	namespaceResolver, err := netns.NewResolver(config.Probe, manager, statsdClient, tcResolver)
	if err != nil {
		return nil, err
	}

	var sbomResolver *sbom.Resolver

	if config.RuntimeSecurity.SBOMResolverEnabled {
		sbomResolver, err = sbom.NewSBOMResolver(config.RuntimeSecurity, statsdClient)
		if err != nil {
			return nil, err
		}
	}

	tagsResolver := tags.NewResolver(config.Probe)
	cgroupsResolver, err := cgroup.NewResolver(tagsResolver)
	if err != nil {
		return nil, err
	}

	if config.RuntimeSecurity.SBOMResolverEnabled {
		_ = cgroupsResolver.RegisterListener(cgroup.CGroupDeleted, sbomResolver.OnCGroupDeletedEvent)
		_ = cgroupsResolver.RegisterListener(cgroup.WorkloadSelectorResolved, sbomResolver.OnWorkloadSelectorResolvedEvent)
	}

	mountResolver, err := mount.NewResolver(statsdClient, cgroupsResolver, mount.ResolverOpts{UseProcFS: true})
	if err != nil {
		return nil, err
	}

	pathResolver := path.NewResolver(dentryResolver, mountResolver)

	containerResolver := &container.Resolver{}
	processResolver, err := process.NewResolver(manager, config.Probe, statsdClient,
		scrubber, containerResolver, mountResolver, cgroupsResolver, userGroupResolver, timeResolver, pathResolver, process.NewResolverOpts(config.Probe.EnvsWithValue))
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		manager:           manager,
		MountResolver:     mountResolver,
		ContainerResolver: containerResolver,
		TimeResolver:      timeResolver,
		UserGroupResolver: userGroupResolver,
		TagsResolver:      tagsResolver,
		DentryResolver:    dentryResolver,
		NamespaceResolver: namespaceResolver,
		CGroupResolver:    cgroupsResolver,
		TCResolver:        tcResolver,
		ProcessResolver:   processResolver,
		PathResolver:      pathResolver,
		SBOMResolver:      sbomResolver,
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

	r.CGroupResolver.Start(ctx)
	if r.SBOMResolver != nil {
		r.SBOMResolver.Start(ctx)
	}
	return r.NamespaceResolver.Start(ctx)
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	if err := r.snapshot(); err != nil {
		return fmt.Errorf("unable to snapshot processes: %w", err)
	}

	r.ProcessResolver.SetState(process.Snapshotted)
	r.NamespaceResolver.SetState(process.Snapshotted)

	selinuxStatusMap, err := managerhelper.Map(r.manager, "selinux_enforce_status")
	if err != nil {
		return fmt.Errorf("unable to snapshot SELinux: %w", err)
	}

	if err := selinux.SnapshotSELinux(selinuxStatusMap); err != nil {
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

		if process.IsKThread(uint32(ppid), uint32(proc.Pid)) {
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
