// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultKillActionFlushDelay = 2 * time.Second
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

// KillAndReport kill and report
func (p *ProcessKiller) KillAndReport(scope string, signal string, ev *model.Event, killFnc func(pid uint32, sig uint32) error) {
	entry, exists := ev.ResolveProcessCacheEntry()
	if !exists {
		return
	}

	switch scope {
	case "container", "process":
	default:
		scope = "process"
	}

	pids, err := p.getPids(scope, ev, entry)
	if err != nil {
		log.Errorf("unable to kill: %s", err)
		return
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
