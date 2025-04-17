// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"errors"
	"math"
	"os"
	"syscall"

	psutil "github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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
		"/opt/datadog-agent/bin/datadog-cluster-agent",
		// installer
		"/opt/datadog-packages/datadog-agent/*/bin/agent/agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/trace-agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/security-agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/process-agent",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/system-probe",
		"/opt/datadog-packages/datadog-agent/*/embedded/bin/cws-instrumentation",
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
	killFunc func(pid, sig uint32) error
}

// NewProcessKillerOS returns a ProcessKillerOS
func NewProcessKillerOS(f func(pid, sig uint32) error) ProcessKillerOS {
	return &ProcessKillerLinux{
		killFunc: f,
	}
}

// Kill tries to kill from userspace
func (p *ProcessKillerLinux) Kill(sig uint32, pc *killContext) error {

	// check path
	exePathLink := utils.ProcExePath(uint32(pc.pid))
	exePath, err := os.Readlink(exePathLink)
	if err != nil {
		return errors.New("process not found in procfs")
	}
	if exePath != pc.path {
		return errors.New("paths don't match")
	}

	// check timestamp
	if pc.createdAt != 0 {
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
	}

	return syscall.Kill(pc.pid, syscall.Signal(sig))
}

// TODO: do a better job than returning only the direct lineage
func (p *ProcessKillerLinux) getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]killContext, error) {
	if entry.ContainerID != "" && scope == "container" {
		pcs := []killContext{}
		pids, paths := entry.GetContainerPIDs()
		l := min(len(pids), len(paths))
		for i := 0; i < l; i++ {
			pid := pids[i]
			path := paths[i]
			if pid < 1 || path == "" {
				continue
			}
			proc, err := psutil.NewProcess(int32(pid))
			if err != nil {
				continue
			}
			createdAt, err := proc.CreateTime()
			if err != nil {
				continue
			}
			pcs = append(pcs, killContext{
				pid:       int(pid),
				path:      path,
				createdAt: uint64(createdAt),
			})
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
