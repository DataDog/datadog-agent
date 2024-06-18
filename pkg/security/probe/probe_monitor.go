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

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/approver"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/cgroups"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/discarder"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/syscalls"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// EBPFMonitors regroups all the work we want to do to monitor the probes we pushed in the kernel
type EBPFMonitors struct {
	ebpfProbe *EBPFProbe

	eventStreamMonitor *eventstream.Monitor
	runtimeMonitor     *runtime.Monitor
	discarderMonitor   *discarder.Monitor
	cgroupsMonitor     *cgroups.Monitor
	approverMonitor    *approver.Monitor
	syscallsMonitor    *syscalls.Monitor
}

// NewEBPFMonitors returns a new instance of a ProbeMonitor
func NewEBPFMonitors(p *EBPFProbe) *EBPFMonitors {
	return &EBPFMonitors{
		ebpfProbe: p,
	}
}

// Init initializes the monitor
func (m *EBPFMonitors) Init() error {
	var err error
	p := m.ebpfProbe

	// instantiate a new event statistics monitor
	m.eventStreamMonitor, err = eventstream.NewEventStreamMonitor(p.config.Probe, p.Erpc, p.Manager, p.statsdClient, p.onEventLost, p.UseRingBuffers())
	if err != nil {
		return fmt.Errorf("couldn't create the events statistics monitor: %w", err)
	}

	if p.config.Probe.RuntimeMonitor {
		m.runtimeMonitor = runtime.NewRuntimeMonitor(p.statsdClient)
	}

	m.discarderMonitor, err = discarder.NewDiscarderMonitor(p.Manager, p.statsdClient)
	if err != nil {
		return fmt.Errorf("couldn't create the discarder monitor: %w", err)
	}
	m.approverMonitor, err = approver.NewApproverMonitor(p.Manager, p.statsdClient)
	if err != nil {
		return fmt.Errorf("couldn't create the approver monitor: %w", err)
	}

	if p.opts.SyscallsMonitorEnabled {
		m.syscallsMonitor, err = syscalls.NewSyscallsMonitor(p.Manager, p.statsdClient)
		if err != nil {
			return fmt.Errorf("couldn't create the approver monitor: %w", err)
		}
	}

	m.cgroupsMonitor = cgroups.NewCgroupsMonitor(p.statsdClient, p.Resolvers.CGroupResolver)

	return nil
}

// GetEventStreamMonitor returns the perf buffer monitor
func (m *EBPFMonitors) GetEventStreamMonitor() *eventstream.Monitor {
	return m.eventStreamMonitor
}

// SendStats sends statistics about the probe to Datadog
func (m *EBPFMonitors) SendStats() error {
	if resolvers := m.ebpfProbe.Resolvers; resolvers != nil {
		if err := resolvers.ProcessResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send process_resolver stats: %w", err)
		}

		if err := resolvers.DentryResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send process_resolver stats: %w", err)
		}

		if err := resolvers.NamespaceResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send namespace_resolver stats: %w", err)
		}

		if err := resolvers.MountResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send mount_resolver stats: %w", err)
		}

		if resolvers.SBOMResolver != nil {
			if err := resolvers.SBOMResolver.SendStats(); err != nil {
				return fmt.Errorf("failed to send sbom_resolver stats: %w", err)
			}
		}

		if resolvers.HashResolver != nil {
			if err := resolvers.HashResolver.SendStats(); err != nil {
				return fmt.Errorf("failed to send hash_resolver stats: %w", err)
			}
		}
	}

	if err := m.eventStreamMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send events stats: %w", err)
	}

	if m.ebpfProbe.config.Probe.RuntimeMonitor {
		if err := m.runtimeMonitor.SendStats(); err != nil {
			return fmt.Errorf("failed to send runtime monitor stats: %w", err)
		}
	}

	if err := m.discarderMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send discarder stats: %w", err)
	}

	if err := m.cgroupsMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send cgroups stats: %w", err)
	}

	if err := m.approverMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send evaluation set stats: %w", err)
	}

	if m.ebpfProbe.opts.SyscallsMonitorEnabled {
		if err := m.syscallsMonitor.SendStats(); err != nil {
			return fmt.Errorf("failed to send evaluation set stats: %w", err)
		}
	}

	return nil
}

// ProcessEvent processes an event through the various monitors and controllers of the probe
func (m *EBPFMonitors) ProcessEvent(event *model.Event) {
	if !m.ebpfProbe.config.RuntimeSecurity.InternalMonitoringEnabled {
		return
	}

	// handle event errors
	if event.Error == nil {
		return
	}

	var notCritical *path.ErrPathResolutionNotCritical
	if errors.As(event.Error, &notCritical) {
		return
	}

	var pathErr *path.ErrPathResolution
	if errors.As(event.Error, &pathErr) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.AbnormalPathRuleID, events.AbnormalPathRuleDesc, event, pathErr.Err),
		)
	}

	if errors.Is(event.Error, model.ErrNoProcessContext) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.NoProcessContextErrorRuleID, events.NoProcessContextErrorRuleDesc, event, event.Error),
		)
	}

	var brokenLineageErr *model.ErrProcessBrokenLineage
	if errors.As(event.Error, &brokenLineageErr) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.BrokenProcessLineageErrorRuleID, events.BrokenProcessLineageErrorRuleDesc, event, event.Error),
		)
	}

	var argsEnvsErr *model.ErrProcessArgsEnvsResolution
	if errors.As(event.Error, &argsEnvsErr) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.NoProcessArgsEnvsErrorRuleID, events.NoProcessArgsEnvsErrorRuleDesc, event, event.Error),
		)
	}
}
