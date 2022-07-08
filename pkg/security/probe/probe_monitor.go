// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Monitor regroups all the work we want to do to monitor the probes we pushed in the kernel
type Monitor struct {
	probe *Probe

	loadController      *LoadController
	perfBufferMonitor   *PerfBufferMonitor
	syscallMonitor      *SyscallMonitor
	activityDumpManager *ActivityDumpManager
	runtimeMonitor      *RuntimeMonitor
	discarderMonitor    *DiscarderMonitor
}

// NewMonitor returns a new instance of a ProbeMonitor
func NewMonitor(p *Probe) (*Monitor, error) {
	var err error
	m := &Monitor{
		probe: p,
	}

	// instantiate a new load controller
	m.loadController, err = NewLoadController(p)
	if err != nil {
		return nil, err
	}

	// instantiate a new event statistics monitor
	m.perfBufferMonitor, err = NewPerfBufferMonitor(p)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create the events statistics monitor")
	}

	if p.config.ActivityDumpEnabled {
		m.activityDumpManager, err = NewActivityDumpManager(p)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't create the activity dump manager")
		}
	}

	// create a new syscall monitor if requested
	if p.config.SyscallMonitor {
		m.syscallMonitor, err = NewSyscallMonitor(p.manager)
		if err != nil {
			return nil, err
		}
	}

	if p.config.RuntimeMonitor {
		m.runtimeMonitor = NewRuntimeMonitor(p.statsdClient)
	}

	m.discarderMonitor, err = NewDiscarderMonitor(p)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't create the discarder monitor")
	}

	return m, nil
}

// GetPerfBufferMonitor returns the perf buffer monitor
func (m *Monitor) GetPerfBufferMonitor() *PerfBufferMonitor {
	return m.perfBufferMonitor
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
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (m *Monitor) SendStats() error {
	// delay between to send in order to reduce the statsd pool presure
	const delay = time.Second

	if m.syscallMonitor != nil {
		if err := m.syscallMonitor.SendStats(m.probe.statsdClient); err != nil {
			return errors.Wrap(err, "failed to send syscall monitor stats")
		}
	}
	time.Sleep(delay)

	if resolvers := m.probe.GetResolvers(); resolvers != nil {
		if err := resolvers.ProcessResolver.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send process_resolver stats")
		}
		time.Sleep(delay)

		if err := resolvers.DentryResolver.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send process_resolver stats")
		}
		if err := resolvers.NamespaceResolver.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send namespace_resolver stats")
		}
	}

	if err := m.perfBufferMonitor.SendStats(); err != nil {
		return errors.Wrap(err, "failed to send events stats")
	}
	time.Sleep(delay)

	if err := m.loadController.SendStats(); err != nil {
		return errors.Wrap(err, "failed to send load controller stats")
	}

	if m.activityDumpManager != nil {
		if err := m.activityDumpManager.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send activity dump maanger stats")
		}
	}

	if m.probe.config.RuntimeMonitor {
		if err := m.runtimeMonitor.SendStats(); err != nil {
			return errors.Wrap(err, "failed to send runtime monitor stats")
		}
	}

	if err := m.discarderMonitor.SendStats(); err != nil {
		return errors.Wrap(err, "failed to send discarder stats")
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
func (m *Monitor) ProcessEvent(event *Event) {
	m.loadController.Count(event)

	// Look for an unresolved path
	if err := event.GetPathResolutionError(); err != nil {
		m.probe.DispatchCustomEvent(
			NewAbnormalPathEvent(event, err),
		)
	} else {
		if m.activityDumpManager != nil {
			m.activityDumpManager.ProcessEvent(event)
		}
	}
}

// RuleSetLoadedReport represents the rule and the custom event related to a RuleSetLoaded event, ready to be dispatched
type RuleSetLoadedReport struct {
	Rule  *rules.Rule
	Event *CustomEvent
}

// PrepareRuleSetLoadedReport prepares a report of new loaded ruleset
func (m *Monitor) PrepareRuleSetLoadedReport(ruleSet *rules.RuleSet, err *multierror.Error) RuleSetLoadedReport {
	r, ev := NewRuleSetLoadedEvent(ruleSet, err)
	return RuleSetLoadedReport{Rule: r, Event: ev}
}

// ReportRuleSetLoaded reports to Datadog that new ruleset was loaded
func (m *Monitor) ReportRuleSetLoaded(report RuleSetLoadedReport) {
	if err := m.probe.statsdClient.Count(metrics.MetricRuleSetLoaded, 1, []string{}, 1.0); err != nil {
		log.Error(errors.Wrap(err, "failed to send ruleset_loaded metric"))
	}

	m.probe.DispatchCustomEvent(report.Rule, report.Event)
}

// SelfTestReport represents the rule and the custom event related to a SelfTest event; ready to be dispatched
type SelfTestReport struct {
	Rule  *rules.Rule
	Event *CustomEvent
}

// ReportSelfTest reports to Datadog that a self test was performed
func (m *Monitor) ReportSelfTest(success []string, fails []string) {
	// send metric with number of success and fails
	tags := []string{
		fmt.Sprintf("success:%d", len(success)),
		fmt.Sprintf("fails:%d", len(fails)),
	}
	if err := m.probe.statsdClient.Count(metrics.MetricSelfTest, 1, tags, 1.0); err != nil {
		log.Error(errors.Wrap(err, "failed to send self_test metric"))
	}

	// send the custom event with the list of succeed and failed self tests
	r, ev := NewSelfTestEvent(success, fails)
	report := SelfTestReport{Rule: r, Event: ev}
	m.probe.DispatchCustomEvent(report.Rule, report.Event)
}

// ErrActivityDumpManagerDisabled is returned when the activity dump manager is disabled
var ErrActivityDumpManagerDisabled = errors.New("ActivityDumpManager is disabled")

// DumpActivity handles an activity dump request
func (m *Monitor) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if !m.probe.config.ActivityDumpEnabled {
		return &api.ActivityDumpMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.DumpActivity(params)
}

// ListActivityDumps returns the list of active dumps
func (m *Monitor) ListActivityDumps(params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if !m.probe.config.ActivityDumpEnabled {
		return &api.ActivityDumpListMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.ListActivityDumps(params)
}

// StopActivityDump stops an active activity dump
func (m *Monitor) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if !m.probe.config.ActivityDumpEnabled {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.StopActivityDump(params)
}

// GenerateTranscoding encodes an activity dump following the input parameters
func (m *Monitor) GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if !m.probe.config.ActivityDumpEnabled {
		return &api.TranscodingRequestMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}
	return m.activityDumpManager.TranscodingRequest(params)
}
