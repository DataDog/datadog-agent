// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"
)

// Monitor regroups all the work we want to do to monitor the probes we pushed in the kernel
type Monitor struct {
	probe  *Probe
	client *statsd.Client

	loadController    *LoadController
	perfBufferMonitor *PerfBufferMonitor
	syscallMonitor    *SyscallMonitor
	reordererMonitor  *ReordererMonitor
}

// NewMonitor returns a new instance of a ProbeMonitor
func NewMonitor(p *Probe, client *statsd.Client) (*Monitor, error) {
	var err error
	m := &Monitor{
		probe:  p,
		client: client,
	}

	// instantiate a new load controller
	m.loadController, err = NewLoadController(p, client)
	if err != nil {
		return nil, err
	}

	// instantiate a new event statistics monitor
	m.perfBufferMonitor, err = NewPerfBufferMonitor(p, client)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create the events statistics monitor")
	}

	m.reordererMonitor, err = NewReOrderMonitor(p, client)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create the reorder monitor")
	}

	// create a new syscall monitor if requested
	if p.config.SyscallMonitor {
		m.syscallMonitor, err = NewSyscallMonitor(p.manager)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

// GetPerfBufferMonitor returns the perf buffer monitor
func (m *Monitor) GetPerfBufferMonitor() *PerfBufferMonitor {
	return m.perfBufferMonitor
}

// Start triggers the goroutine of all the underlying controllers and monitors of the Monitor
func (m *Monitor) Start(ctx context.Context) error {
	go m.loadController.Start(ctx)
	go m.reordererMonitor.Start(ctx)
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (m *Monitor) SendStats() error {
	if m.syscallMonitor != nil {
		if err := m.syscallMonitor.SendStats(m.client); err != nil {
			return errors.Wrap(err, "failed to send syscall monitor stats")
		}
	}

	if resolvers := m.probe.GetResolvers(); resolvers != nil {
		if err := resolvers.ProcessResolver.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send process_resolver stats")
		}
		if err := resolvers.DentryResolver.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send process_resolver stats")
		}
	}

	if err := m.perfBufferMonitor.SendStats(); err != nil {
		return errors.Wrap(err, "failed to send events stats")
	}

	return nil
}

// GetStats returns Stats according to the system-probe module format
func (m *Monitor) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var syscalls *SyscallStats
	var err error

	if m.syscallMonitor != nil {
		syscalls, err = m.syscallMonitor.GetStats()
	}

	stats["events"] = map[string]interface{}{
		"perf_buffer": 0,
		"syscalls":    syscalls,
	}
	return stats, err
}

// ProcessEvent processes an event through the various monitors and controllers of the probe
func (m *Monitor) ProcessEvent(event *Event, size uint64, CPU int, perfMap *manager.PerfMap) {
	m.loadController.Count(event)

	// Look for an unresolved path
	if err := event.GetPathResolutionError(); err != nil {
		m.probe.DispatchCustomEvent(
			NewAbnormalPathEvent(event, err),
		)
	}
}

// ProcessLostEvent processes a lost event through the various monitors and controllers of the probe
func (m *Monitor) ProcessLostEvent(count uint64, cpu int, perfMap *manager.PerfMap) {
	log.Tracef("lost %d events\n", count)
	m.perfBufferMonitor.CountLostEvent(count, perfMap, cpu)
}

// ReportRuleSetLoaded reports to Datadog that new ruleset was loaded
func (m *Monitor) ReportRuleSetLoaded(ruleSet *rules.RuleSet, err *multierror.Error) {
	if err := m.client.Count(metrics.MetricRuleSetLoaded, 1, []string{}, 1.0); err != nil {
		log.Error(errors.Wrap(err, "failed to send ruleset_loaded metric"))
	}

	m.probe.DispatchCustomEvent(
		NewRuleSetLoadedEvent(ruleSet, err),
	)
}
