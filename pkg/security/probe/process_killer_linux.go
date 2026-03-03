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
	"syscall"

	psutil "github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var (
	// list of binaries that can't be killed
	binariesExcluded = []string{
		// package / image
		"/opt/datadog-agent/bin/agent/agent",
		"/opt/datadog-agent/embedded/bin/trace-agent",
		"/opt/datadog-agent/embedded/bin/security-agent",
		"/opt/datadog-agent/embedded/bin/process-agent",
		"/opt/datadog-agent/embedded/bin/system-probe",
		"/opt/datadog-agent/embedded/bin/cws-instrumentation",
		"/opt/datadog-agent/embedded/bin/privateactionrunner",
		"/opt/datadog-agent/bin/datadog-cluster-agent",
		// installer
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
}

// NewProcessKillerOS returns a ProcessKillerOS
func NewProcessKillerOS(killFunc func(pid, sig uint32) error, cgroupResolver *cgroup.Resolver) ProcessKillerOS {
	return &ProcessKillerLinux{
		killFunc:       killFunc,
		cgroupResolver: cgroupResolver,
	}
}

// Kill tries to kill from userspace
func (p *ProcessKillerLinux) Kill(sig uint32, pc *killContext) error {
	proc, err := psutil.NewProcess(int32(pc.pid))
	if err != nil {
		return errors.New("process not found in procfs")
	}
	createdAt, err := proc.CreateTime()
	if err != err {
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
			var cacheEntry *cgroupModel.CacheEntry
			if !entry.ContainerContext.IsNull() {
				cacheEntry = p.cgroupResolver.GetCacheEntryContainerID(entry.ContainerContext.ContainerID)
				if cacheEntry == nil {
					return pcs, errors.New("container not found")
				}
			} else {
				cacheEntry = p.cgroupResolver.GetCacheEntryByCgroupID(entry.CGroup.CGroupID)
				if cacheEntry == nil {
					return pcs, errors.New("cgroup not found")
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
