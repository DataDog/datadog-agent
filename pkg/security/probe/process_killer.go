// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"syscall"
	"time"

	psutil "github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	userSpaceKillWithinMillis = 2000
)

// ProcessKiller defines a process killer structure
type ProcessKiller struct {
	sync.Mutex

	pendingReports []*KillActionReport
}

// NewProcessKiller returns a new ProcessKiller
func NewProcessKiller() *ProcessKiller {
	return &ProcessKiller{}
}

// AddPendingReports add a pending reports
func (p *ProcessKiller) AddPendingReports(report *KillActionReport) {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = append(p.pendingReports, report)
}

// FlushPendingReports flush pending reports
func (p *ProcessKiller) FlushPendingReports() {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *KillActionReport) bool {
		report.Lock()
		defer report.Unlock()

		if time.Now().After(report.KilledAt.Add(defaultKillActionFlushDelay)) {
			report.resolved = true
			return true
		}
		return false
	})
}

// HandleProcessExited handles process exited events
func (p *ProcessKiller) HandleProcessExited(event *model.Event) {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *KillActionReport) bool {
		report.Lock()
		defer report.Unlock()

		if report.Pid == event.ProcessContext.Pid {
			report.ExitedAt = event.ProcessContext.ExitTime
			report.resolved = true
			return true
		}
		return false
	})
}

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

// KillAndReport kill and report
func (p *ProcessKiller) KillAndReport(scope string, signal string, ev *model.Event, killFnc func(pid uint32, sig uint32) error) {
	entry, exists := ev.ResolveProcessCacheEntry()
	if !exists {
		return
	}

	var pids []uint32

	if entry.ContainerID != "" && scope == "container" {
		pids = entry.GetContainerPIDs()
		scope = "container"
	} else {
		pids = []uint32{ev.ProcessContext.Pid}
		scope = "process"
	}

	sig := model.SignalConstants[signal]

	killedAt := time.Now()
	for _, pid := range pids {
		if pid <= 1 || pid == utils.Getpid() {
			continue
		}

		log.Debugf("requesting signal %s to be sent to %d", signal, pid)

		if err := killFnc(uint32(pid), uint32(sig)); err != nil {
			seclog.Debugf("failed to kill process %d: %s", pid, err)
		}
	}

	p.Lock()
	defer p.Unlock()

	report := &KillActionReport{
		Scope:      scope,
		Signal:     signal,
		Pid:        ev.ProcessContext.Pid,
		CreatedAt:  ev.ProcessContext.ExecTime,
		DetectedAt: ev.ResolveEventTime(),
		KilledAt:   killedAt,
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)
}
