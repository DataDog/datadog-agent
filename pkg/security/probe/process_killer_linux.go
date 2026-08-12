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
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
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
	// cgroup is set when the process belongs to a cgroup that may be killed in a single
	// operation, instead of signalling each of its processes.
	cgroup cgroupKillTarget
}

// cgroupKillTarget identifies a cgroup to kill in a single operation. The inode is carried along
// with the ID so the killer can check that the path it resolves to is still the expected cgroup.
type cgroupKillTarget struct {
	id    containerutils.CGroupID
	inode uint64
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
func NewProcessKillerOS(killFunc func(pid, sig uint32) error, cgroupResolver *cgroup.Resolver, cgroupKillEnabled bool) ProcessKillerOS {
	// Without a cgroup resolver there is no cgroup to resolve a kill scope to, so don't pay the
	// cost of looking up the cgroup mount points.
	var cgroupKiller *cgroupKiller
	if cgroupKillEnabled && cgroupResolver != nil {
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

// Kill sends the given signal to the given processes. Processes that belong to a cgroup that can
// be killed at once are killed with a single write to its cgroup.kill, the others one by one.
func (p *ProcessKillerLinux) Kill(sig uint32, kcs []killContext) ([]uint32, []uint32) {
	var failedPids, killedPids []uint32

	// cgroup.kill only ever delivers SIGKILL, anything else has to be sent per process.
	remaining := kcs
	if p.cgroupKiller != nil && sig == uint32(unix.SIGKILL) {
		remaining = nil
		for _, group := range groupByCgroup(kcs) {
			if group.target.id == "" {
				remaining = append(remaining, group.kcs...)
				continue
			}

			// A cgroup kill that fails killed nothing, so its processes can safely be killed
			// one by one instead.
			if err := p.cgroupKiller.kill(group.target); err != nil {
				seclog.Warnf("unable to kill cgroup `%s` in a single operation, falling back to killing each process: %s", group.target.id, err)
				remaining = append(remaining, group.kcs...)
				continue
			}

			seclog.Debugf("killed cgroup `%s` and its %d known processes with a single operation", group.target.id, len(group.kcs))

			// The kernel killed by cgroup membership, so every process the cgroup held is reported
			// as killed without the create-time check killProcess does below. A pid that exited
			// between being enumerated and the kill, and whose exit was never seen so that
			// HandleProcessExited could drop it from a queued report, is reported as killed even
			// though a process that has recycled its pid in another cgroup was left alone. The
			// report therefore means the cgroup was emptied, rather than that each of these pids
			// was individually confirmed killed.
			for _, kc := range group.kcs {
				killedPids = append(killedPids, uint32(kc.pid))
			}
		}
	}

	for _, kc := range remaining {
		if err := p.killProcess(sig, &kc); err != nil {
			seclog.Debugf("failed to kill process %d: %s.", kc.pid, err)
			failedPids = append(failedPids, uint32(kc.pid))
		} else {
			killedPids = append(killedPids, uint32(kc.pid))
		}
	}

	return failedPids, killedPids
}

// cgroupKillGroup is a set of processes that share the same cgroup kill target.
type cgroupKillGroup struct {
	target cgroupKillTarget
	kcs    []killContext
}

// groupByCgroup groups the given processes by the cgroup they can be killed with, keeping the ones
// that have no such cgroup in a single group with an empty target. Groups are only created for
// processes that are actually in them, so a group can never be empty: killing a cgroup we have no
// process to report as killed would take down a workload we know nothing about.
func groupByCgroup(kcs []killContext) []*cgroupKillGroup {
	var groups []*cgroupKillGroup
	byCgroupID := make(map[containerutils.CGroupID]*cgroupKillGroup)

	for _, kc := range kcs {
		group, ok := byCgroupID[kc.cgroup.id]
		if !ok {
			group = &cgroupKillGroup{target: kc.cgroup}
			byCgroupID[kc.cgroup.id] = group
			groups = append(groups, group)
		}
		group.kcs = append(group.kcs, kc)
	}

	return groups
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

// killProcess tries to kill a single process from userspace
func (p *ProcessKillerLinux) killProcess(sig uint32, pc *killContext) error {
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

			// Remember the cgroup these processes belong to, so that Kill can take them all down
			// with a single write instead of signalling each of them.
			var target cgroupKillTarget
			if p.cgroupKiller != nil {
				if id, inode := cacheEntry.GetCGroupID(), cacheEntry.GetCGroupInode(); id != "" && inode != 0 {
					target = cgroupKillTarget{id: id, inode: inode}
				}
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
					cgroup:    target,
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
