// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"context"
	json "encoding/json"
	"errors"
	"io"
	"os/exec"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// DarwinProbe defines a macOS probe
type DarwinProbe struct {
	probe         *Probe
	event         *model.Event
	resolvers     *resolvers.Resolvers
	fieldHandlers *FieldHandlers
	ctx           context.Context
	cancelFnc     context.CancelFunc
}

// NewDarwinProbe returns a new darwin probe
func NewDarwinProbe(p *Probe, config *config.Config, opts Opts) (*DarwinProbe, error) {
	resolvers, err := resolvers.NewResolvers(config, opts.StatsdClient, p.scrubber)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	return &DarwinProbe{
		probe:         p,
		resolvers:     resolvers,
		fieldHandlers: &FieldHandlers{},
		ctx:           ctx,
		cancelFnc:     cancelFnc,
	}, nil
}

// Setup sets up the probe
func (dp *DarwinProbe) Setup() error { return nil }

// Init initializes the probe
func (dp *DarwinProbe) Init() error { return nil }

// Start starts the probe
func (dp *DarwinProbe) Start() error {
	cmd := exec.Command("/usr/bin/eslogger", "exec")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(stdout)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		<-dp.ctx.Done()
		if err := cmd.Process.Kill(); err != nil {
			log.Errorf("error killing eslogger process: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			log.Errorf("error waiting for eslogger process: %v", err)
		}
	}()

	go func() {
		var value ESEvent
		for {
			err := decoder.Decode(&value)
			if err == io.EOF {
				break
			}

			dp.pushEvent(&value)
		}
	}()

	return nil
}

func (dp *DarwinProbe) pushEvent(esev *ESEvent) {
	event := dp.zeroEvent()
	event.Type = uint32(model.ExecEventType)

	pid := esev.Event.Exec.Target.AuditToken.Pid
	ppid := esev.Event.Exec.Target.ParentAuditToken.Pid

	// TODO(paulcacheux): add parent pid
	pce, err := dp.resolvers.ProcessResolver.AddNewEntry(pid, ppid, esev.Event.Exec.Target.Executable.Path, esev.Event.Exec.Args)
	if err != nil {
		log.Errorf("error in resolver %v", err)
		return
	}

	event.Exec.Process = &pce.Process
	event.ProcessCacheEntry = pce
	event.ProcessContext = &pce.ProcessContext
	dp.DispatchEvent(event)
}

func (dp *DarwinProbe) zeroEvent() *model.Event {
	dp.event.Zero()
	dp.event.FieldHandlers = dp.fieldHandlers
	return dp.event
}

// DispatchEvent sends an event to the probe event handler
func (dp *DarwinProbe) DispatchEvent(event *model.Event) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, nil)
		return eventJSON, event.GetEventType(), err
	})

	// send event to wildcard handlers, like the CWS rule engine, first
	dp.probe.sendEventToWildcardHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	dp.probe.sendEventToSpecificEventTypeHandlers(event)
}

// Stop stops the probe
func (dp *DarwinProbe) Stop() {
	dp.cancelFnc()
}

// SendStats sends stats to the probe statsd client
func (dp *DarwinProbe) SendStats() error { return nil }

// Snapshot collects data on the current state of the system
func (dp *DarwinProbe) Snapshot() error {
	return dp.resolvers.Snapshot()
}

// Close closes the probe
func (dp *DarwinProbe) Close() error { return nil }

// NewModel returns a new model
func (dp *DarwinProbe) NewModel() *model.Model {
	return NewDarwinModel()
}

// DumpDiscarders dumps discarders
func (dp *DarwinProbe) DumpDiscarders() (string, error) {
	return "", errors.New("not supported")
}

// FlushDiscarders flushes discarders
func (dp *DarwinProbe) FlushDiscarders() error { return nil }

// ApplyRuleSet applies a rule set
func (dp *DarwinProbe) ApplyRuleSet(_ *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return &kfilters.ApplyRuleSetReport{}, nil
}

// OnNewDiscarder is called when a new discarder is created
func (dp *DarwinProbe) OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType) {
}

// HandleActions handles actions
func (dp *DarwinProbe) HandleActions(_ *eval.Context, _ *rules.Rule) {}

// NewEvent returns a new event
func (dp *DarwinProbe) NewEvent() *model.Event {
	return NewDarwinEvent(dp.fieldHandlers)
}

// GetFieldHandlers returns the field handlers
func (dp *DarwinProbe) GetFieldHandlers() model.FieldHandlers {
	return dp.fieldHandlers
}

// DumpProcessCache dumps the process cache
func (dp *DarwinProbe) DumpProcessCache(_ bool) (string, error) { return "", nil }

// AddDiscarderPushedCallback adds a discarder pushed callback
func (dp *DarwinProbe) AddDiscarderPushedCallback(_ DiscarderPushedCallback) {}

// GetEventTags returns the event tags
func (dp *DarwinProbe) GetEventTags(_ string) []string { return nil }

// Origin returns origin
func (p *Probe) Origin() string {
	return ""
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, opts Opts, wmeta optional.Option[workloadmeta.Component]) (*Probe, error) {
	opts.normalize()

	p := &Probe{
		Opts:         opts,
		Config:       config,
		StatsdClient: opts.StatsdClient,
		scrubber:     newProcScrubber(config.Probe.CustomSensitiveWords),
	}

	pp, err := NewDarwinProbe(p, config, opts)
	if err != nil {
		return nil, err
	}
	p.PlatformProbe = pp

	return p, nil
}

// ESEvent is the event sent by eslogger
type ESEvent struct {
	Event struct {
		Exec struct {
			Args   []string `json:"args"`
			Target struct {
				AuditToken struct {
					Pid uint32 `json:"pid"`
				} `json:"audit_token"`
				ParentAuditToken struct {
					Pid uint32 `json:"pid"`
				} `json:"parent_audit_token"`
				Executable struct {
					Path string `json:"path"`
				} `json:"executable"`
			} `json:"target"`
		} `json:"exec"`
	} `json:"event"`
}
