// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package probe holds probe related files
package probe

import (
	"context"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/procmon"
)

type PlatformProbe struct {
	pm      *procmon.WinProcmon
	onStart chan *procmon.ProcessStartNotification
	onStop  chan *procmon.ProcessStopNotification
}

// Init initializes the probe
func (p *Probe) Init() error {
	p.startTime = time.Now()

	pm, err := procmon.NewWinProcMon(p.onStart, p.onStop)
	if err != nil {
		return err
	}
	p.pm = pm

	return nil
}

// Setup the runtime security probe
func (p *Probe) Setup() error {
	return nil
}

// Stop the probe
func (p *Probe) Stop() {}

// Start processing events
func (p *Probe) Start() error {
	log.Infof("Windows probe started")
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			var err error
			var e *model.ProcessCacheEntry
			ev := p.zeroEvent()
			select {
			case <-p.ctx.Done():
				return
			case start := <-p.onStart:
				log.Infof("Received start %v", start)
				// this doesn't take into account the possibility of
				// PID collision
				e, err = p.resolvers.ProcessResolver.AddNewProcessEntry(process.Pid(start.Pid), start.ImageFile, start.CmdLine)
				if err != nil {
					// count the error and
					log.Infof("error in resolver %v", err)
					continue
				}
				ev.Type = uint32(model.ExecEventType)
				if e != nil {
					ev.Exec.Process = &e.Process
				}
			case stop := <-p.onStop:
				log.Infof("Received stop %v", stop)
				e = p.resolvers.ProcessResolver.GetProcessEntry(process.Pid(stop.Pid))
				defer p.resolvers.ProcessResolver.DeleteProcessEntry(process.Pid(stop.Pid))

				ev.Type = uint32(model.ExitEventType)
				if e != nil {
					ev.Exit.Process = &e.Process
				}
			}

			if e != nil {
				// use ProcessCacheEntry process context as process context
				ev.ProcessCacheEntry = e
				ev.ProcessContext = &e.ProcessContext

				p.DispatchEvent(ev)
			}

		}
	}()
	return p.pm.Start()
}

// DispatchEvent sends an event to the probe event handler
func (p *Probe) DispatchEvent(event *model.Event) {

	// send event to wildcard handlers, like the CWS rule engine, first
	p.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.sendEventToSpecificEventTypeHandlers(event)

}

func (p *Probe) sendEventToWildcardHandlers(event *model.Event) {
	for _, handler := range p.fullAccessEventHandlers[model.UnknownEventType] {
		handler.HandleEvent(event)
	}
}

func (p *Probe) sendEventToSpecificEventTypeHandlers(event *model.Event) {
	for _, handler := range p.eventHandlers[event.GetEventType()] {
		handler.HandleEvent(handler.Copy(event))
	}
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	//return p.resolvers.Snapshot()
	return nil
}

// Close the probe
func (p *Probe) Close() error {
	p.pm.Stop()
	p.cancelFnc()
	p.wg.Wait()
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	//p.resolvers.TCResolver.SendTCProgramsStats(p.StatsdClient)
	//
	//return p.monitor.SendStats()
	return nil
}

// GetDebugStats returns the debug stats
func (p *Probe) GetDebugStats() map[string]interface{} {
	debug := map[string]interface{}{
		"start_time": p.startTime.String(),
	}
	return debug
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts) (*Probe, error) {
	opts.normalize()

	ctx, cancel := context.WithCancel(context.Background())

	p := &Probe{
		Opts:                 opts,
		Config:               config,
		ctx:                  ctx,
		cancelFnc:            cancel,
		StatsdClient:         opts.StatsdClient,
		discarderRateLimiter: rate.NewLimiter(rate.Every(time.Second/5), 100),
		PlatformProbe: PlatformProbe{
			onStart: make(chan *procmon.ProcessStartNotification),
			onStop:  make(chan *procmon.ProcessStopNotification),
		},
	}

	p.scrubber = procutil.NewDefaultDataScrubber()
	p.scrubber.AddCustomSensitiveWords(config.Probe.CustomSensitiveWords)

	resolvers, err := resolvers.NewResolvers(config, p.StatsdClient, p.scrubber)
	if err != nil {
		return nil, err
	}
	p.resolvers = resolvers

	p.fieldHandlers = &FieldHandlers{resolvers: resolvers}

	p.event = NewEvent(p.fieldHandlers)

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// Does nothing if on windows
func (p *Probe) PlaySnapshot() {
}

// OnNewDiscarder is called when a new discarder is found. We currently don't generate discarders on Windows.
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, ev *model.Event, field eval.Field, eventType eval.EventType) {
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *Probe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return kfilters.NewApplyRuleSetReport(p.Config.Probe, rs)
}
