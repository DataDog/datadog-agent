// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Monitor regroups all the work we want to do to monitor the probes we pushed in the kernel
type Monitor struct {
	probe *Probe

	loadController    *LoadController
	perfBufferMonitor *PerfBufferMonitor
	runtimeMonitor    *RuntimeMonitor
	discarderMonitor  *DiscarderMonitor
	cgroupsMonitor    *CgroupsMonitor
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

	// instantiate a new load controller
	m.loadController, err = NewLoadController(p)
	if err != nil {
		return err
	}

	// instantiate a new event statistics monitor
	m.perfBufferMonitor, err = NewPerfBufferMonitor(p, p.onPerfEventLost)
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

	m.cgroupsMonitor = NewCgroupsMonitor(p.StatsdClient, p.resolvers.CGroupResolver)

	return nil
}

// GetPerfBufferMonitor returns the perf buffer monitor
func (m *Monitor) GetPerfBufferMonitor() *PerfBufferMonitor {
	return m.perfBufferMonitor
}

// Start triggers the goroutine of all the underlying controllers and monitors of the Monitor
func (m *Monitor) Start(ctx context.Context, wg *sync.WaitGroup) error {
	wg.Add(1)

	go m.loadController.Start(ctx, wg)

	return nil
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

	if err := m.perfBufferMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send events stats: %w", err)
	}
	time.Sleep(delay)

	if err := m.loadController.SendStats(); err != nil {
		return fmt.Errorf("failed to send load controller stats: %w", err)
	}

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

	return nil
}

// ProcessEvent processes an event through the various monitors and controllers of the probe
func (m *Monitor) ProcessEvent(event *model.Event) {
	m.loadController.Count(event)

	// Look for an unresolved path
	if err := event.PathResolutionError; err != nil {
		var notCritical *path.ErrPathResolutionNotCritical
		if !errors.As(err, &notCritical) {
			m.probe.DispatchCustomEvent(
				NewAbnormalPathEvent(event, m.probe, err),
			)
		}
	}
}
