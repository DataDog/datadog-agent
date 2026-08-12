// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"syscall"

	psutil "github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// getBinariesExcluded returns the list of binaries that can't be killed.
// Paths are built dynamically based on the configured install path.
func getBinariesExcluded() []string {
	installPath := defaultpaths.GetInstallPath()

	return []string{
		// package / image - dynamic paths based on install location
		filepath.Join(installPath, "bin/agent/agent"),
		filepath.Join(installPath, "embedded/bin/trace-agent"),
		filepath.Join(installPath, "embedded/bin/security-agent"),
		filepath.Join(installPath, "embedded/bin/process-agent"),
		filepath.Join(installPath, "embedded/bin/system-probe"),
		filepath.Join(installPath, "embedded/bin/cws-instrumentation"),
		filepath.Join(installPath, "embedded/bin/privateactionrunner"),
		filepath.Join(installPath, "bin/datadog-cluster-agent"),
		// installer - these use wildcards and remain hard-coded
		"/opt/datadog-packages/datadog-agent/*/bin/agent/agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/trace-agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/security-agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/process-agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/system-probe",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/cws-instrumentation",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/privateactionrunner",
		"/opt/datadog-packages/datadog-agent/*/bin/datadog-cluster-agent",
		"/opt/datadog-packages/datadog-installer/*/bin/installer/installer",
	}
}

var (
	// binariesExcluded is populated at init time from getBinariesExcluded()
	binariesExcluded = getBinariesExcluded()
)

type killContext struct {
	createdAt uint64
	pid       int
	path      string
	// containerID string?? TODO: be able to specify the containerID to kill
}

const (
	killWithinMillis = 2000
)

// ProcessKillerLinux defines the process kill linux implementation
type ProcessKillerLinux struct {
	killFunc       func(pid, sig uint32) error
	cgroupResolver *cgroup.Resolver
	// cgroupKiller is nil when the host can't kill cgroups in a single operation, in which case
	// processes are killed one by one.
	cgroupKiller *cgroupKiller
}

// NewProcessKillerOS returns a ProcessKillerOS
func NewProcessKillerOS(killFunc func(pid, sig uint32) error, cgroupResolver *cgroup.Resolver) ProcessKillerOS {
	// Without a cgroup resolver there is no cgroup to resolve a kill scope to, so don't pay the
	// cost of looking up the cgroup mount points.
	var cgroupKiller *cgroupKiller
	if cgroupResolver != nil {
		var err error
		if cgroupKiller, err = newCgroupKiller(); err != nil {
			seclog.Infof("cgroups will be killed process by process: %s", err)
		}
	}

	return &ProcessKillerLinux{
		killFunc:       killFunc,
		cgroupResolver: cgroupResolver,
		cgroupKiller:   cgroupKiller,
	}
}

// KillCgroup kills every process of the target cgroup, and of its descendant cgroups, at once
func (p *ProcessKillerLinux) KillCgroup(target cgroupKillTarget) error {
	if p.cgroupKiller == nil {
		return errCgroupKillUnavailable
	}
	return p.cgroupKiller.kill(target)
}

// getCgroupKillTarget returns the cgroup to kill in a single operation for the given scope
func (p *ProcessKillerLinux) getCgroupKillTarget(scope string, entry *model.ProcessCacheEntry) (cgroupKillTarget, bool) {
	if p.cgroupKiller == nil || (scope != "container" && scope != "cgroup") {
		return cgroupKillTarget{}, false
	}

	cacheEntry, err := p.getCgroupCacheEntry(entry)
	if err != nil {
		return cgroupKillTarget{}, false
	}

	target := cgroupKillTarget{
		id:    cacheEntry.GetCGroupID(),
		inode: cacheEntry.GetCGroupInode(),
	}
	if target.id == "" || target.inode == 0 {
		return cgroupKillTarget{}, false
	}
	return target, true
}

// getCgroupCacheEntry returns the cgroup the given process belongs to
func (p *ProcessKillerLinux) getCgroupCacheEntry(entry *model.ProcessCacheEntry) (*cgroupModel.CacheEntry, error) {
	if p.cgroupResolver == nil {
		return nil, errors.New("no cgroup resolver")
	}

	if !entry.ContainerContext.IsNull() {
		cacheEntry := p.cgroupResolver.GetCacheEntryContainerID(entry.ContainerContext.ContainerID)
		if cacheEntry == nil {
			return nil, errors.New("container not found")
		}
		return cacheEntry, nil
	}

	cacheEntry := p.cgroupResolver.GetCacheEntryByCgroupID(entry.CGroup.CGroupID)
	if cacheEntry == nil {
		return nil, errors.New("cgroup not found")
	}
	return cacheEntry, nil
}

// Kill tries to kill from userspace
func (p *ProcessKillerLinux) Kill(sig uint32, pc *killContext) error {
	proc, err := psutil.NewProcess(int32(pc.pid))
	if err != nil {
		return errors.New("process not found in procfs")
	}
	createdAt, err := proc.CreateTime()
	if err != nil {
		return errors.New("process not found in procfs")
	}
	if math.Abs(float64(pc.createdAt-uint64(createdAt))) > killWithinMillis {
		return errors.New("create at timestamps don't match")
	}

	err = syscall.Kill(pc.pid, syscall.Signal(sig))
	if err != nil && p.killFunc != nil {
		err = p.killFunc(uint32(pc.pid), sig)
	}
	if err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pc.pid, err)
	}
	return nil
}

func (p *ProcessKillerLinux) getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]killContext, error) {
	if scope == "container" || scope == "cgroup" {
		pcs := []killContext{}

		// Use the CGroupResolver to get all PIDs of the container
		if p.cgroupResolver != nil {
			cacheEntry, err := p.getCgroupCacheEntry(entry)
			if err != nil {
				return pcs, err
			}

			for _, pid := range cacheEntry.GetPIDs() {
				if pid < 1 {
					continue
				}
				proc, err := psutil.NewProcess(int32(pid))
				if err != nil {
					continue
				}
				createdAt, err := proc.CreateTime()
				if err != nil || createdAt == 0 {
					continue
				}
				// Get the executable path from procfs
				exe, err := proc.Exe()
				if err != nil {
					continue
				}
				pcs = append(pcs, killContext{
					pid:       int(pid),
					path:      exe,
					createdAt: uint64(createdAt),
				})
			}
		}
		return pcs, nil
	}

	return []killContext{
		{
			createdAt: uint64(ev.ProcessContext.ExecTime.UnixMilli()),
			pid:       int(ev.ProcessContext.Pid),
			path:      ev.ProcessContext.FileEvent.PathnameStr,
		},
	}, nil
}
