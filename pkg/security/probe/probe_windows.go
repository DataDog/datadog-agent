// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/procmon"
	"golang.org/x/time/rate"
)

type PlatformProbe struct {
	pm      *procmon.WinProcmon
	onStart chan *procmon.ProcessStartNotification
	onStop  chan *procmon.ProcessStopNotification
}

// Init initializes the probe
func (p *Probe) Init() error {
	pm, err := procmon.NewWinProcMon(p.onStart, p.onStop)
	if err != nil {
		return nil
	}
	p.pm = pm
	return nil
}

// Setup the runtime security probe
func (p *Probe) Setup() error {
	return nil
}

var eventZero model.Event

func (p *Probe) zeroEvent() *model.Event {
	*p.event = eventZero
	//p.event.FieldHandlers = p.fieldHandlers
	return p.event
}

// Start processing events
func (p *Probe) Start() error {
	log.Infof("Windows probe started")
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			var e *ProcessEntry
			var err error
			ev := &model.Event{}
			select {
			case <-p.ctx.Done():
				return
			case start := <-p.onStart:
				log.Infof("Received start %v", start)
				// this doesn't take into account the possibility of
				// PID collision
				e, err = p.resolvers.ProcessResolver.AddNewProcessEntry(Pid(start.Pid), start.ImageFile, start.CmdLine)
				if err != nil {
					// count the error and
					log.Infof("error in resolver %v", err)
					continue
				}
				ev.Type = uint32(model.ExecEventType)
			case stop := <-p.onStop:
				log.Infof("Received stop %v", stop)
				e = p.resolvers.ProcessResolver.GetProcessEntry(Pid(stop.Pid))
				defer p.resolvers.ProcessResolver.DeleteProcessEntry(Pid(stop.Pid))
				ev.Type = uint32(model.ExitEventType)
			}

			if e != nil {
				proc := &model.Process{}
				proc.PIDContext.Pid = uint32(e.Pid)
				proc.Argv0 = e.ImageFile
				proc.Argv = e.CommandLine
				ev.Exec.Process = proc
				p.DispatchEvent(ev)
			}

		}
	}()
	p.pm.Start()
	return nil
}

// DispatchEvent sends an event to the probe event handler
func (p *Probe) DispatchEvent(event *model.Event) {

	// send wildcard first
	for _, handler := range p.eventHandlers[model.UnknownEventType] {
		handler.HandleEvent(event)
	}

	// send specific event
	for _, handler := range p.eventHandlers[event.GetEventType()] {
		handler.HandleEvent(event)
	}

}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return nil
	//return p.resolvers.Snapshot()
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
	// TODO(Will): add manager state
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
		event:                &model.Event{},
		PlatformProbe: PlatformProbe{
			onStart: make(chan *procmon.ProcessStartNotification),
			onStop:  make(chan *procmon.ProcessStopNotification),
		},
	}
	resolvers, err := NewResolvers(config, p)
	if err != nil {
		return nil, err
	}
	p.resolvers = resolvers
	return p, nil
}
