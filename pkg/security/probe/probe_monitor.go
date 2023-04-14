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

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// Monitor regroups all the work we want to do to monitor the probes we pushed in the kernel
type Monitor struct {
	probe *Probe

	loadController         *LoadController
	perfBufferMonitor      *PerfBufferMonitor
	activityDumpManager    *dump.ActivityDumpManager
	securityProfileManager *profile.SecurityProfileManager
	runtimeMonitor         *RuntimeMonitor
	discarderMonitor       *DiscarderMonitor
	cgroupsMonitor         *CgroupsMonitor
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
	m.perfBufferMonitor, err = NewPerfBufferMonitor(p)
	if err != nil {
		return fmt.Errorf("couldn't create the events statistics monitor: %w", err)
	}

	if p.IsActivityDumpEnabled() {
		m.activityDumpManager, err = dump.NewActivityDumpManager(p.Config, p.StatsdClient, func() *model.Event { return NewEvent(p.fieldHandlers) }, p.resolvers.ProcessResolver, p.resolvers.TimeResolver, p.resolvers.TagsResolver, p.kernelVersion, p.scrubber, p.Manager)
		if err != nil {
			return fmt.Errorf("couldn't create the activity dump manager: %w", err)
		}
	}

	if p.Config.RuntimeSecurity.SecurityProfileEnabled {
		m.securityProfileManager, err = profile.NewSecurityProfileManager(p.Config, p.StatsdClient, p.resolvers.CGroupResolver, p.Manager)
		if err != nil {
			return fmt.Errorf("couldn't create the security profile manager: %w", err)
		}
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

// GetActivityDumpManager returns the activity dump manager
func (m *Monitor) GetActivityDumpManager() *dump.ActivityDumpManager {
	return m.activityDumpManager
}

// Start triggers the goroutine of all the underlying controllers and monitors of the Monitor
func (m *Monitor) Start(ctx context.Context, wg *sync.WaitGroup) error {
	delta := 1
	if m.activityDumpManager != nil {
		delta++
	}
	wg.Add(delta)

	go m.loadController.Start(ctx, wg)

	if m.activityDumpManager != nil {
		go m.activityDumpManager.Start(ctx, wg)
	}
	if m.securityProfileManager != nil {
		go m.securityProfileManager.Start(ctx)
	}
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

	if m.activityDumpManager != nil {
		if err := m.activityDumpManager.SendStats(); err != nil {
			return fmt.Errorf("failed to send activity dump manager stats: %w", err)
		}
	}

	if m.securityProfileManager != nil {
		if err := m.securityProfileManager.SendStats(); err != nil {
			return fmt.Errorf("failed to send security profile manager stats: %w", err)
		}
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
	} else {
		if m.activityDumpManager != nil {
			m.activityDumpManager.ProcessEvent(event)
		}
	}
}

// ErrActivityDumpManagerDisabled is returned when the activity dump manager is disabled
var ErrActivityDumpManagerDisabled = errors.New("ActivityDumpManager is disabled")

// DumpActivity handles an activity dump request
func (m *Monitor) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if !m.probe.IsActivityDumpEnabled() {
		return &api.ActivityDumpMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.DumpActivity(params)
}

// ListActivityDumps returns the list of active dumps
func (m *Monitor) ListActivityDumps(params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if !m.probe.IsActivityDumpEnabled() {
		return &api.ActivityDumpListMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.ListActivityDumps(params)
}

// StopActivityDump stops an active activity dump
func (m *Monitor) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if !m.probe.IsActivityDumpEnabled() {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.StopActivityDump(params)
}

// GenerateTranscoding encodes an activity dump following the input parameters
func (m *Monitor) GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if !m.probe.IsActivityDumpEnabled() {
		return &api.TranscodingRequestMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.TranscodingRequest(params)
}

func (m *Monitor) GetActivityDumpTracedEventTypes() []model.EventType {
	return m.probe.Config.RuntimeSecurity.ActivityDumpTracedEventTypes
}
