// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"syscall"

	psutil "github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	userSpaceKillWithinMillis = 2000
)

var (
	// list of binaries that can't be killed
	binariesExcluded = []string{
		"/opt/datadog-agent/bin/agent/agent",
		"/opt/datadog-agent/embedded/bin/trace-agent",
		"/opt/datadog-agent/embedded/bin/security-agent",
		"/opt/datadog-agent/embedded/bin/process-agent",
		"/opt/datadog-agent/embedded/bin/system-probe",
		"/opt/datadog-agent/embedded/bin/cws-instrumentation",
		"/opt/datadog-agent/bin/datadog-cluster-agent",
	}
)

// KillFromUserspace tries to kill from userspace
func (p *ProcessKiller) KillFromUserspace(pid uint32, sig uint32, ev *model.Event) error {
	proc, err := psutil.NewProcess(int32(pid))
	if err != nil {
		return errors.New("process not found in procfs")
	}

	name, err := proc.Name()
	if err != nil {
		return errors.New("process not found in procfs")
	}

	createdAt, err := proc.CreateTime()
	if err != nil {
		return errors.New("process not found in procfs")
	}
	evCreatedAt := ev.ProcessContext.ExecTime.UnixMilli()

	within := uint64(evCreatedAt) >= uint64(createdAt-userSpaceKillWithinMillis) && uint64(evCreatedAt) <= uint64(createdAt+userSpaceKillWithinMillis)

	if !within || ev.ProcessContext.Comm != name {
		return fmt.Errorf("not sharing the same namespace: %s/%s", ev.ProcessContext.Comm, name)
	}

	return syscall.Kill(int(pid), syscall.Signal(sig))
}

func (p *ProcessKiller) getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]uint32, []string, error) {
	var (
		pids  []uint32
		paths []string
	)

	if entry.ContainerID != "" && scope == "container" {
		pids, paths = entry.GetContainerPIDs()
	} else {
		pids = []uint32{ev.ProcessContext.Pid}
		paths = []string{ev.ProcessContext.FileEvent.PathnameStr}
	}
	return pids, paths, nil
}
