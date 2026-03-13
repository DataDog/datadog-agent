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
	"sort"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dns"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/envvars"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/file"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/selinux"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sign"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/syscallctx"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usersessions"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

// EBPFResolvers holds the list of the event attribute resolvers
type EBPFResolvers struct {
	manager              *manager.Manager
	MountResolver        mount.ResolverInterface
	TimeResolver         *ktime.Resolver
	UserGroupResolver    *usergroup.Resolver
	TagsResolver         *tags.LinuxResolver
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
	DNSResolver          *dns.Resolver
	FileMetadataResolver *file.Resolver
	SignatureResolver    *sign.Resolver

	SnapshotUsingListmount bool
}

// NewEBPFResolvers creates a new instance of EBPFResolvers
func NewEBPFResolvers(config *config.Config, manager *manager.Manager, statsdClient statsd.ClientInterface, scrubber *utils.Scrubber, eRPC *erpc.ERPC, opts Opts) (*EBPFResolvers, error) {
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

	cgroupsResolver, err := cgroup.NewResolver(statsdClient, nil, dentryResolver)
	if err != nil {
		return nil, err
	}

	// Create version resolver function that uses SBOM resolver if available
	var versionResolver func(servicePath string) string
	if config.RuntimeSecurity.SBOMResolverEnabled && sbomResolver != nil {
		versionResolver = func(servicePath string) string {
			if pkg := sbomResolver.ResolvePackage("", &model.FileEvent{PathnameStr: servicePath}); pkg != nil {
				return pkg.Version
			}
			return ""
		}
	}

	tagsResolver := tags.NewResolver(opts.Tagger, cgroupsResolver, versionResolver)

	userGroupResolver, err := usergroup.NewResolver(cgroupsResolver)
	if err != nil {
		return nil, err
	}

	if config.RuntimeSecurity.SBOMResolverEnabled {
		if err := cgroupsResolver.RegisterListener(cgroup.CGroupDeleted, sbomResolver.OnCGroupDeletedEvent); err != nil {
			return nil, err
		}
		if err := tagsResolver.RegisterListener(tags.WorkloadSelectorResolved, sbomResolver.OnWorkloadSelectorResolvedEvent); err != nil {
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
		resolverOpts := mount.ResolverOpts{
			UseProcFS:              true,
			SnapshotUsingListMount: config.Probe.SnapshotUsingListmount,
		}
		mountResolver, err = mount.NewResolver(statsdClient, cgroupsResolver, dentryResolver, resolverOpts)
		if err != nil {
			return nil, err
		}
		pathResolver = path.NewResolver(dentryResolver, mountResolver)
	} else {
		mountResolver = &mount.NoOpResolver{}
		pathResolver = &path.NoOpResolver{}
	}

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

	var dnsResolver *dns.Resolver
	if config.Probe.DNSResolutionEnabled {
		dnsResolver, err = dns.NewDNSResolver(config.Probe, statsdClient)
		if err != nil {
			return nil, err
		}
	}

	hashResolver, err := hash.NewResolver(config.RuntimeSecurity, statsdClient, cgroupsResolver)
	if err != nil {
		return nil, err
	}

	userSessionsResolver, err := usersessions.NewResolver(config.RuntimeSecurity.UserSessionsCacheSize, config.RuntimeSecurity.SSHUserSessionsEnabled)
	if err != nil {
		return nil, err
	}

	fileMetadataResolver, err := file.NewResolver(config.RuntimeSecurity, statsdClient, &file.Opt{CgroupResolver: cgroupsResolver})
	if err != nil {
		return nil, err
	}

	processResolver, err := process.NewEBPFResolver(manager, config.Probe, statsdClient,
		scrubber, mountResolver, cgroupsResolver, userGroupResolver, timeResolver, pathResolver, envVarsResolver, userSessionsResolver, processOpts)
	if err != nil {
		return nil, err
	}

	resolvers := &EBPFResolvers{
		manager:                manager,
		MountResolver:          mountResolver,
		TimeResolver:           timeResolver,
		UserGroupResolver:      userGroupResolver,
		TagsResolver:           tagsResolver,
		DentryResolver:         dentryResolver,
		NamespaceResolver:      namespaceResolver,
		CGroupResolver:         cgroupsResolver,
		TCResolver:             tcResolver,
		ProcessResolver:        processResolver,
		PathResolver:           pathResolver,
		SBOMResolver:           sbomResolver,
		HashResolver:           hashResolver,
		UserSessionsResolver:   userSessionsResolver,
		SyscallCtxResolver:     syscallctx.NewResolver(),
		DNSResolver:            dnsResolver,
		FileMetadataResolver:   fileMetadataResolver,
		SnapshotUsingListmount: config.Probe.SnapshotUsingListmount,
		SignatureResolver:      sign.NewSignatureResolver(),
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

	err = r.MountResolver.SyncCache()

	if err != nil {
		seclog.Errorf("failed to sync cache from listmount: %v", err)
		r.SnapshotUsingListmount = false
	}

	// Sync the namespace cache
	r.NamespaceResolver.SyncCache()

	for _, proc := range processes {
		ppid, err := proc.Ppid()
		if err != nil {
			continue
		}

		pid := uint32(proc.Pid)

		if process.IsKThread(uint32(ppid), pid) {
			continue
		}

		// Sync the process cache
		r.ProcessResolver.SyncCache(proc)
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
	// clean up the user sessions resolver goroutine and resources
	if r.UserSessionsResolver != nil {
		r.UserSessionsResolver.Close()
	}
	return nil
}
