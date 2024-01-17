// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/windowsdriver/procmon"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// WindowsProbe defines a Windows probe
type WindowsProbe struct {
	Resolvers *resolvers.Resolvers

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	// internals
	event         *model.Event
	ctx           context.Context
	cancelFnc     context.CancelFunc
	wg            sync.WaitGroup
	probe         *Probe
	fieldHandlers *FieldHandlers
	pm            *procmon.WinProcmon
	onStart       chan *procmon.ProcessStartNotification
	onStop        chan *procmon.ProcessStopNotification
	onError       chan bool
}

// Init initializes the probe
func (p *WindowsProbe) Init() error {
	pm, err := procmon.NewWinProcMon(p.onStart, p.onStop, p.onError)
	if err != nil {
		return err
	}
	p.pm = pm

	return nil
}

// Setup the runtime security probe
func (p *WindowsProbe) Setup() error {
	return nil
}

// Stop the probe
func (p *WindowsProbe) Stop() {
	p.pm.Stop()
}

// Start processing events
func (p *WindowsProbe) Start() error {

	log.Infof("Windows probe started")
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		for {
			var pce *model.ProcessCacheEntry
			var err error
			ev := p.zeroEvent()
			var pidToCleanup uint32

			select {
			case <-p.ctx.Done():
				return

			case <-p.onError:
				// in this case, we got some sort of error that the underlying
				// subsystem can't recover from.  Need to initiate some sort of cleanup

			case start := <-p.onStart:
				pid := process.Pid(start.Pid)
				if pid == 0 {
					// TODO this shouldn't happen
					continue
				}

				log.Debugf("Received start %v", start)

				// TODO
				// handle new fields
				// CreatingPRocessId
				// CreatingThreadId
				// OwnerSidString
				if start.RequiredSize != 0 {
					// in this case, the command line and/or the image file might not be filled in
					// depending upon how much space was needed.

					// potential actions
					// - just log/count the error and keep going
					// - restart underlying procmon with larger buffer size, at least if error keeps occurring
					log.Warnf("insufficient buffer size %v", start.RequiredSize)

				}

				pce, err = p.Resolvers.ProcessResolver.AddNewEntry(pid, uint32(start.PPid), start.ImageFile, start.CmdLine)
				if err != nil {
					log.Errorf("error in resolver %v", err)
					continue
				}
				ev.Type = uint32(model.ExecEventType)
				ev.Exec.Process = &pce.Process
			case stop := <-p.onStop:
				pid := process.Pid(stop.Pid)
				if pid == 0 {
					// TODO this shouldn't happen
					continue
				}
				log.Infof("Received stop %v", stop)

				pce = p.Resolvers.ProcessResolver.GetEntry(pid)
				pidToCleanup = pid

				ev.Type = uint32(model.ExitEventType)
				if pce == nil {
					log.Errorf("unable to resolve pid %d", pid)
					continue
				}
				ev.Exit.Process = &pce.Process
			}

			if pce == nil {
				continue
			}

			// use ProcessCacheEntry process context as process context
			ev.ProcessCacheEntry = pce
			ev.ProcessContext = &pce.ProcessContext

			p.DispatchEvent(ev)

			if pidToCleanup != 0 {
				p.Resolvers.ProcessResolver.DeleteEntry(pidToCleanup, time.Now())
				pidToCleanup = 0
			}
		}
	}()
	return p.pm.Start()
}

// DispatchEvent sends an event to the probe event handler
func (p *WindowsProbe) DispatchEvent(event *model.Event) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	p.probe.sendEventToSpecificEventTypeHandlers(event)

}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *WindowsProbe) Snapshot() error {
	return p.Resolvers.Snapshot()
}

// Close the probe
func (p *WindowsProbe) Close() error {
	p.pm.Stop()
	p.cancelFnc()
	p.wg.Wait()
	return nil
}

// SendStats sends statistics about the probe to Datadog
func (p *WindowsProbe) SendStats() error {
	return nil
}

// NewWindowsProbe instantiates a new runtime security agent probe
func NewWindowsProbe(probe *Probe, config *config.Config, opts Opts) (*WindowsProbe, error) {
	ctx, cancelFnc := context.WithCancel(context.Background())

	p := &WindowsProbe{
		probe:        probe,
		config:       config,
		opts:         opts,
		statsdClient: opts.StatsdClient,
		ctx:          ctx,
		cancelFnc:    cancelFnc,
		onStart:      make(chan *procmon.ProcessStartNotification),
		onStop:       make(chan *procmon.ProcessStopNotification),
		onError:      make(chan bool),
	}

	var err error
	p.Resolvers, err = resolvers.NewResolvers(config, p.statsdClient, probe.scrubber)
	if err != nil {
		return nil, err
	}

	p.fieldHandlers = &FieldHandlers{resolvers: p.Resolvers}

	p.event = p.NewEvent()

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *WindowsProbe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return kfilters.NewApplyRuleSetReport(p.config.Probe, rs)
}

// FlushDiscarders invalidates all the discarders
func (p *WindowsProbe) FlushDiscarders() error {
	return nil
}

// OnNewDiscarder handles discarders
func (p *WindowsProbe) OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType) {
}

// NewModel returns a new Model
func (p *WindowsProbe) NewModel() *model.Model {
	return NewWindowsModel(p)
}

// DumpDiscarders dump the discarders
func (p *WindowsProbe) DumpDiscarders() (string, error) {
	return "", errors.New("not supported")
}

// GetFieldHandlers returns the field handlers
func (p *WindowsProbe) GetFieldHandlers() model.FieldHandlers {
	return p.fieldHandlers
}

// DumpProcessCache dumps the process cache
func (p *WindowsProbe) DumpProcessCache(_ bool) (string, error) {
	return "", errors.New("not supported")
}

// NewEvent returns a new event
func (p *WindowsProbe) NewEvent() *model.Event {
	return NewWindowsEvent(p.fieldHandlers)
}

// HandleActions executes the actions of a triggered rule
func (p *WindowsProbe) HandleActions(_ *eval.Context, _ *rules.Rule) {}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *WindowsProbe) AddDiscarderPushedCallback(_ DiscarderPushedCallback) {}

// GetEventTags returns the event tags
func (p *WindowsProbe) GetEventTags(_ string) []string {
	return nil
}

func (p *WindowsProbe) zeroEvent() *model.Event {
	p.event.Zero()
	p.event.FieldHandlers = p.fieldHandlers
	return p.event
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts) (*Probe, error) {
	opts.normalize()

	p := &Probe{
		Opts:         opts,
		Config:       config,
		StatsdClient: opts.StatsdClient,
		scrubber:     newProcScrubber(config.Probe.CustomSensitiveWords),
	}

	pp, err := NewWindowsProbe(p, config, opts)
	if err != nil {
		return nil, err
	}
	p.PlatformProbe = pp

	return p, nil
}
