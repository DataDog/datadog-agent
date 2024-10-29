// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package resolvers holds resolvers related files
package resolvers

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/container"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/envvars"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/selinux"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/syscallctx"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EBPFResolvers holds the list of the event attribute resolvers
type EBPFResolvers struct {
	manager              *manager.Manager
	MountResolver        mount.ResolverInterface
	ContainerResolver    *container.Resolver
	TimeResolver         *ktime.Resolver
	UserGroupResolver    *usergroup.Resolver
	TagsResolver         tags.Resolver
	DentryResolver       *dentry.Resolver
	ProcessResolver      *process.EBPFResolver
	NamespaceResolver    *netns.Resolver
	CGroupResolver       *cgroup.Resolver
	TCResolver           *tc.Resolver
	PathResolver         path.ResolverInterface
	SBOMResolver         *sbom.Resolver
	HashResolver         *hash.Resolver
	UserSessionsResolver *usersessions.Resolver
	SyscallCtxResolver   *syscallctx.Resolver
}

// NewEBPFResolvers creates a new instance of EBPFResolvers
func NewEBPFResolvers(config *config.Config, manager *manager.Manager, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber, eRPC *erpc.ERPC, opts Opts, telemetry telemetry.Component) (*EBPFResolvers, error) {
	dentryResolver, err := dentry.NewResolver(config.Probe, statsdClient, eRPC)
	if err != nil {
		return nil, err
	}

	timeResolver, err := ktime.NewResolver()
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

	var tagsResolver tags.Resolver
	if opts.TagsResolver != nil {
		tagsResolver = opts.TagsResolver
	} else {
		tagsResolver = tags.NewResolver(config.Probe, telemetry)
	}

	cgroupsResolver, err := cgroup.NewResolver(tagsResolver)
	if err != nil {
		return nil, err
	}

	userGroupResolver, err := usergroup.NewResolver(cgroupsResolver)
	if err != nil {
		return nil, err
	}

	if config.RuntimeSecurity.SBOMResolverEnabled {
		if err := cgroupsResolver.RegisterListener(cgroup.CGroupDeleted, sbomResolver.OnCGroupDeletedEvent); err != nil {
			return nil, err
		}
		if err := cgroupsResolver.RegisterListener(cgroup.WorkloadSelectorResolved, sbomResolver.OnWorkloadSelectorResolvedEvent); err != nil {
			return nil, err
		}
	}

	if err := cgroupsResolver.RegisterListener(cgroup.CGroupDeleted, userGroupResolver.OnCGroupDeletedEvent); err != nil {
		return nil, err
	}

	var mountResolver mount.ResolverInterface

	var pathResolver path.ResolverInterface
	if opts.PathResolutionEnabled {
		// Force the use of redemption for now, as it seems that the kernel reference counter on mounts used to remove mounts is not working properly.
		// This means that we can remove mount entries that are still in use.
		mountResolver, err = mount.NewResolver(statsdClient, cgroupsResolver, mount.ResolverOpts{UseProcFS: true})
		if err != nil {
			return nil, err
		}
		pathResolver = path.NewResolver(dentryResolver, mountResolver)
	} else {
		mountResolver = &mount.NoOpResolver{}
		pathResolver = &path.NoOpResolver{}
	}
	containerResolver := &container.Resolver{}

	processOpts := process.NewResolverOpts()
	processOpts.WithEnvsValue(config.Probe.EnvsWithValue)
	if opts.TTYFallbackEnabled {
		processOpts.WithTTYFallbackEnabled()
	}
	if opts.EnvVarsResolutionEnabled {
		processOpts.WithEnvsResolutionEnabled()
	}

	var envVarsResolver *envvars.Resolver
	if opts.EnvVarsResolutionEnabled {
		envVarsResolver = envvars.NewEnvVarsResolver(config.Probe)
	}

	processResolver, err := process.NewEBPFResolver(manager, config.Probe, statsdClient,
		scrubber, containerResolver, mountResolver, cgroupsResolver, userGroupResolver, timeResolver, pathResolver, envVarsResolver, processOpts)
	if err != nil {
		return nil, err
	}
	hashResolver, err := hash.NewResolver(config.RuntimeSecurity, statsdClient, cgroupsResolver)
	if err != nil {
		return nil, err
	}

	userSessionsResolver, err := usersessions.NewResolver(config.RuntimeSecurity.UserSessionsCacheSize)
	if err != nil {
		return nil, err
	}

	resolvers := &EBPFResolvers{
		manager:              manager,
		MountResolver:        mountResolver,
		ContainerResolver:    containerResolver,
		TimeResolver:         timeResolver,
		UserGroupResolver:    userGroupResolver,
		TagsResolver:         tagsResolver,
		DentryResolver:       dentryResolver,
		NamespaceResolver:    namespaceResolver,
		CGroupResolver:       cgroupsResolver,
		TCResolver:           tcResolver,
		ProcessResolver:      processResolver,
		PathResolver:         pathResolver,
		SBOMResolver:         sbomResolver,
		HashResolver:         hashResolver,
		UserSessionsResolver: userSessionsResolver,
		SyscallCtxResolver:   syscallctx.NewResolver(),
	}

	return resolvers, nil
}

// Start the resolvers
func (r *EBPFResolvers) Start(ctx context.Context) error {
	if err := r.ProcessResolver.Start(ctx); err != nil {
		return err
	}

	if err := r.TagsResolver.Start(ctx); err != nil {
		return err
	}

	if err := r.DentryResolver.Start(r.manager); err != nil {
		return err
	}

	if err := r.SyscallCtxResolver.Start(r.manager); err != nil {
		return err
	}

	r.CGroupResolver.Start(ctx)
	if r.SBOMResolver != nil {
		if err := r.SBOMResolver.Start(ctx); err != nil {
			return err
		}
	}

	if err := r.UserSessionsResolver.Start(r.manager); err != nil {
		return err
	}
	return r.NamespaceResolver.Start(ctx)
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *EBPFResolvers) Snapshot() error {
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

	return nil
}

// snapshot internal version of Snapshot. Calls the relevant resolvers to sync their caches.
func (r *EBPFResolvers) snapshot() error {
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

		pid := uint32(proc.Pid)

		if process.IsKThread(uint32(ppid), pid) {
			continue
		}

		// Start with the mount resolver because the process resolver might need it to resolve paths
		if err = r.MountResolver.SyncCache(pid); err != nil {
			if !os.IsNotExist(err) {
				log.Debugf("snapshot failed for %d: couldn't sync mount points: %s", proc.Pid, err)
			}
			continue
		}

		// Sync the process cache
		r.ProcessResolver.SyncCache(proc)

		// Sync the namespace cache
		r.NamespaceResolver.SyncCache(pid)
	}

	return nil
}

// Close cleans up any underlying resolver that requires a cleanup
func (r *EBPFResolvers) Close() error {
	// clean up the handles in netns resolver
	r.NamespaceResolver.Close()

	// clean up the dentry resolver eRPC segment
	if err := r.DentryResolver.Close(); err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}
