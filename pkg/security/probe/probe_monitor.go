// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Monitor regroups all the work we want to do to monitor the probes we pushed in the kernel
type Monitor struct {
	probe *Probe

	eventStreamMonitor *eventstream.EventStreamMonitor
	runtimeMonitor     *RuntimeMonitor
	discarderMonitor   *DiscarderMonitor
	cgroupsMonitor     *CgroupsMonitor
	approverMonitor    *ApproverMonitor
}

// NewMonitor returns a new instance of a ProbeMonitor
func NewMonitor(p *Probe) *Monitor {
	return &Monitor{
		probe: p,
	}
}

// Init initializes the monitor
func (m *Monitor) Init() error {
	var err error
	p := m.probe

	// instantiate a new event statistics monitor
	m.eventStreamMonitor, err = eventstream.NewEventStreamMonitor(p.Config.Probe, p.Erpc, p.Manager, p.StatsdClient, p.onEventLost, p.UseRingBuffers())
	if err != nil {
		return fmt.Errorf("couldn't create the events statistics monitor: %w", err)
	}

	if p.Config.Probe.RuntimeMonitor {
		m.runtimeMonitor = NewRuntimeMonitor(p.StatsdClient)
	}

	m.discarderMonitor, err = NewDiscarderMonitor(p.Manager, p.StatsdClient)
	if err != nil {
		return fmt.Errorf("couldn't create the discarder monitor: %w", err)
	}
	m.approverMonitor, err = NewApproverMonitor(p.Manager, p.StatsdClient)
	if err != nil {
		return fmt.Errorf("couldn't create the approver monitor: %w", err)
	}

	m.cgroupsMonitor = NewCgroupsMonitor(p.StatsdClient, p.resolvers.CGroupResolver)

	return nil
}

// GetPerfBufferMonitor returns the perf buffer monitor
func (m *Monitor) GetEventStreamMonitor() *eventstream.EventStreamMonitor {
	return m.eventStreamMonitor
}

// SendStats sends statistics about the probe to Datadog
func (m *Monitor) SendStats() error {
	// delay between two send in order to reduce the statsd pool presure
	const delay = time.Second
	time.Sleep(delay)

	if resolvers := m.probe.GetResolvers(); resolvers != nil {
		if err := resolvers.ProcessResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send process_resolver stats: %w", err)
		}
		time.Sleep(delay)

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
	}

	if err := m.eventStreamMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send events stats: %w", err)
	}
	time.Sleep(delay)

	if m.probe.Config.Probe.RuntimeMonitor {
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

	return nil
}

// ProcessEvent processes an event through the various monitors and controllers of the probe
func (m *Monitor) ProcessEvent(event *model.Event) {
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
		m.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.AbnormalPathRuleID, events.AbnormalPathRuleDesc, event, m.probe, pathErr.Err),
		)
		return
	}

	var processContextErr *ErrNoProcessContext
	if errors.As(event.Error, &processContextErr) {
		m.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.NoProcessContextErrorRuleID, events.NoProcessContextErrorRuleDesc, event, m.probe, event.Error),
		)
		return
	}

	var brokenLineageErr *ErrProcessBrokenLineage
	if errors.As(event.Error, &brokenLineageErr) {
		m.probe.DispatchCustomEvent(
			NewAbnormalEvent(events.BrokenProcessLineageErrorRuleID, events.BrokenProcessLineageErrorRuleDesc, event, m.probe, event.Error),
		)
		return
	}
}
