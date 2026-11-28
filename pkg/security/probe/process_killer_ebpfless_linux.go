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
	"math"
	"syscall"
	"time"

	psutil "github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	ebpflessTracerKillFallbackDelay = 3 * time.Second
	ebpflessFlushReportsInterval    = time.Second
)

type ebpflessKillScope struct {
	nsID        uint64
	containerID containerutils.ContainerID
	cgroupID    containerutils.CGroupID
}

// ProcessKillerEBPFLess implements ProcessKillerOS for eBPF-less mode.
// Both container and cgroup scopes kill every live process tracked for that container/cgroup
// except the cws-instrumentation tracer so it can emit exit events and exit cleanly (stopping
// the container/task). If the tracer has not disconnected within fallbackDelay (e.g. due to a
// racing fork), SIGKILL is sent to the tracer as a fallback.
type ProcessKillerEBPFLess struct {
	probe         *EBPFLessProbe
	selfNSID      uint64
	selfPID       uint32
	fallbackDelay time.Duration

	getProcInfoFn func(pid uint32) (createdAt uint64, exe string, err error)
	killProcessFn func(sig uint32, pc *killContext) error
	killTracerFn  func(pid uint32, expectedCreateTime uint64) error
}

// NewProcessKillerEBPFLess returns a ProcessKillerOS for the eBPF-less probe.
func NewProcessKillerEBPFLess(probe *EBPFLessProbe) *ProcessKillerEBPFLess {
	return &ProcessKillerEBPFLess{
		probe:         probe,
		selfNSID:      getSelfPIDNSID(),
		selfPID:       utils.Getpid(),
		fallbackDelay: ebpflessTracerKillFallbackDelay,
		getProcInfoFn: getProcInfoFromProcFS,
		killProcessFn: killProcessFromProcFS,
		killTracerFn:  killTracerFromProcFS,
	}
}

func getSelfPIDNSID() uint64 {
	var stat syscall.Stat_t
	if err := syscall.Stat("/proc/self/ns/pid", &stat); err != nil {
		return 0
	}
	return stat.Ino
}

func getProcInfoFromProcFS(pid uint32) (uint64, string, error) {
	proc, err := psutil.NewProcess(int32(pid))
	if err != nil {
		return 0, "", err
	}
	createdAt, err := proc.CreateTime()
	if err != nil || createdAt == 0 {
		return 0, "", errors.New("process create time unavailable")
	}
	exe, err := proc.Exe()
	if err != nil {
		return 0, "", err
	}
	return uint64(createdAt), exe, nil
}

func killProcessFromProcFS(sig uint32, pc *killContext) error {
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
	if err := syscall.Kill(pc.pid, syscall.Signal(sig)); err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pc.pid, err)
	}
	return nil
}

func killTracerFromProcFS(pid uint32, expectedCreateTime uint64) error {
	proc, err := psutil.NewProcess(int32(pid))
	if err != nil {
		return errors.New("tracer process not found in procfs")
	}
	createdAt, err := proc.CreateTime()
	if err != nil {
		return errors.New("tracer process not found in procfs")
	}
	if expectedCreateTime != 0 && math.Abs(float64(expectedCreateTime-uint64(createdAt))) > killWithinMillis {
		return errors.New("tracer create at timestamps don't match")
	}
	if err := syscall.Kill(int(pid), syscall.SIGKILL); err != nil {
		return fmt.Errorf("failed to kill tracer process %d: %w", pid, err)
	}
	return nil
}

func (p *ProcessKillerEBPFLess) getProcCreateTime(pid uint32) uint64 {
	if pid <= 1 || p.getProcInfoFn == nil {
		return 0
	}
	createdAt, _, err := p.getProcInfoFn(pid)
	if err != nil {
		return 0
	}
	return createdAt
}

func (p *ProcessKillerEBPFLess) samePIDNamespace(nsID uint64) bool {
	return nsID == 0 || p.selfNSID == 0 || nsID == p.selfNSID
}

// Kill sends the given signal to each process in kcs. For SIGKILL with container/cgroup
// scope, it also arms the tracer fallback timer and racing-fork re-kill on the probe.
func (p *ProcessKillerEBPFLess) Kill(sig uint32, kcs []killContext) ([]uint32, []uint32) {
	var failedPids, killedPids []uint32
	var scopes []ebpflessKillScope
	seenScopes := make(map[ebpflessKillScope]struct{})

	for i := range kcs {
		kc := &kcs[i]
		if err := p.killProcessFn(sig, kc); err != nil {
			seclog.Debugf("failed to kill process %d: %s.", kc.pid, err)
			failedPids = append(failedPids, uint32(kc.pid))
		} else {
			killedPids = append(killedPids, uint32(kc.pid))
		}

		if sig == uint32(unix.SIGKILL) && kc.ebpflessScope != nil {
			scope := *kc.ebpflessScope
			if _, ok := seenScopes[scope]; !ok {
				seenScopes[scope] = struct{}{}
				scopes = append(scopes, scope)
			}
		}
	}

	for _, scope := range scopes {
		p.probe.armContainerKillFallback(scope)
	}

	return failedPids, killedPids
}

func (p *ProcessKillerEBPFLess) getProcesses(scope string, ev *model.Event, entry *model.ProcessCacheEntry) ([]killContext, error) {
	nsID := ev.PIDContext.NSID
	if nsID == 0 {
		nsID = entry.NSID
	}
	if !p.samePIDNamespace(nsID) {
		return nil, fmt.Errorf("cannot kill process in different PID namespace (target=%d, agent=%d)", nsID, p.selfNSID)
	}

	if scope == "container" || scope == "cgroup" {
		containerID := entry.ContainerContext.ContainerID
		if containerID == "" {
			containerID = ev.ProcessContext.Process.ContainerContext.ContainerID
		}
		cgroupID := entry.CGroup.CGroupID
		if cgroupID == "" {
			cgroupID = ev.ProcessContext.Process.CGroup.CGroupID
		}
		if containerID == "" && cgroupID == "" {
			return nil, errors.New("no container or cgroup context to kill")
		}

		tracerPID := p.probe.getClientTracerPID(nsID, containerID, cgroupID)

		pids := make(map[uint32]struct{})
		p.probe.Resolvers.ProcessResolver.Walk(func(e *model.ProcessCacheEntry) {
			if e.Pid <= 1 || e.Pid == p.selfPID || (tracerPID != 0 && e.Pid == tracerPID) {
				return
			}
			if nsID != 0 && e.NSID != 0 && e.NSID != nsID {
				return
			}
			if (containerID != "" && e.ContainerContext.ContainerID == containerID) ||
				(cgroupID != "" && e.CGroup.CGroupID == cgroupID) {
				pids[e.Pid] = struct{}{}
			}
		})

		if evPid := ev.ProcessContext.Pid; evPid > 1 && evPid != p.selfPID && (tracerPID == 0 || evPid != tracerPID) {
			pids[evPid] = struct{}{}
		}

		scopeTarget := &ebpflessKillScope{
			nsID:        nsID,
			containerID: containerID,
			cgroupID:    cgroupID,
		}

		pcs := make([]killContext, 0, len(pids))
		for pid := range pids {
			createdAt, exe, err := p.getProcInfoFn(pid)
			if err != nil || createdAt == 0 {
				continue
			}
			pcs = append(pcs, killContext{
				pid:           int(pid),
				path:          exe,
				createdAt:     createdAt,
				ebpflessScope: scopeTarget,
			})
		}

		if len(pcs) == 0 {
			return nil, errors.New("no live processes found to kill")
		}
		return pcs, nil
	}

	execTime := ev.ProcessContext.ExecTime
	if execTime.IsZero() {
		execTime = ev.ProcessContext.ForkTime
	}

	return []killContext{
		{
			createdAt: uint64(execTime.UnixMilli()),
			pid:       int(ev.ProcessContext.Pid),
			path:      ev.ProcessContext.FileEvent.PathnameStr,
		},
	}, nil
}

// killRacingProcess kills a process that forked or execed while a container/cgroup kill is in progress.
func (p *ProcessKillerEBPFLess) killRacingProcess(nsID uint64, pid uint32) {
	if pid <= 1 || pid == p.selfPID || !p.samePIDNamespace(nsID) {
		return
	}

	createdAt, exe, err := p.getProcInfoFn(pid)
	if err != nil || createdAt == 0 {
		return
	}

	kc := killContext{
		pid:       int(pid),
		path:      exe,
		createdAt: createdAt,
	}

	if allowed, err := p.probe.processKiller.isKillAllowed([]killContext{kc}); !allowed {
		seclog.Debugf("skipping racing process kill for pid %d: %v", pid, err)
		return
	}

	if err := p.killProcessFn(uint32(unix.SIGKILL), &kc); err != nil {
		seclog.Debugf("failed to kill racing process %d: %s", pid, err)
	}
}

func (p *ProcessKillerEBPFLess) killTracer(pid uint32, expectedCreateTime uint64) error {
	if pid <= 1 || pid == p.selfPID {
		return fmt.Errorf("refusing to kill tracer pid %d", pid)
	}
	return p.killTracerFn(pid, expectedCreateTime)
}
