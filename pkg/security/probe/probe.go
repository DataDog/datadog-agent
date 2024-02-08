// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// PlatformProbe defines a platform dependant probe
type PlatformProbe interface {
	Setup() error
	Init() error
	Start() error
	Stop()
	SendStats() error
	Snapshot() error
	Close() error
	NewModel() *model.Model
	DumpDiscarders() (string, error)
	FlushDiscarders() error
	ApplyRuleSet(_ *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error)
	OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType)
	HandleActions(_ *eval.Context, _ *rules.Rule)
	NewEvent() *model.Event
	GetFieldHandlers() model.FieldHandlers
	DumpProcessCache(_ bool) (string, error)
	AddDiscarderPushedCallback(_ DiscarderPushedCallback)
	GetEventTags(_ string) []string
}

// FullAccessEventHandler represents a handler for events sent by the probe that needs access to all the fields in the SECL model
type FullAccessEventHandler interface {
	HandleEvent(event *model.Event)
}

// EventHandler represents a handler for events sent by the probe. This handler makes a copy of the event upon receipt
type EventHandler interface {
	HandleEvent(event any)
	Copy(_ *model.Event) any
}

// CustomEventHandler represents an handler for the custom events sent by the probe
type CustomEventHandler interface {
	HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent)
}

// DiscarderPushedCallback describe the callback used to retrieve pushed discarders information
type DiscarderPushedCallback func(eventType string, event *model.Event, field string)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	PlatformProbe PlatformProbe

	// Constants and configuration
	Opts         Opts
	Config       *config.Config
	StatsdClient statsd.ClientInterface
	startTime    time.Time

	// internals
	scrubber *procutil.DataScrubber

	// Events section
	fullAccessEventHandlers [model.MaxAllEventType][]FullAccessEventHandler
	eventHandlers           [model.MaxAllEventType][]EventHandler
	customEventHandlers     [model.MaxAllEventType][]CustomEventHandler
}

// Init initializes the probe
func (p *Probe) Init() error {
	p.startTime = time.Now()
	return p.PlatformProbe.Init()
}

// Setup the runtime security probe
func (p *Probe) Setup() error {
	return p.PlatformProbe.Setup()
}

// Start plays the snapshot data and then start the event stream
func (p *Probe) Start() error {
	return p.PlatformProbe.Start()
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	return p.PlatformProbe.SendStats()
}

// Close the probe
func (p *Probe) Close() error {
	return p.PlatformProbe.Close()
}

// Stop the probe
func (p *Probe) Stop() {
	p.PlatformProbe.Stop()
}

// FlushDiscarders invalidates all the discarders
func (p *Probe) FlushDiscarders() error {
	seclog.Debugf("Flushing discarders")
	return p.PlatformProbe.FlushDiscarders()
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *Probe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	return p.PlatformProbe.ApplyRuleSet(rs)
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return p.PlatformProbe.Snapshot()
}

// OnNewDiscarder is called when a new discarder is found
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, ev *model.Event, field eval.Field, eventType eval.EventType) {
	p.PlatformProbe.OnNewDiscarder(rs, ev, field, eventType)
}

// DumpDiscarders removes all the discarders
func (p *Probe) DumpDiscarders() (string, error) {
	seclog.Debugf("Dumping discarders")
	return p.PlatformProbe.DumpDiscarders()
}

// DumpProcessCache dump the process cache
func (p *Probe) DumpProcessCache(withArgs bool) (string, error) {
	return p.PlatformProbe.DumpProcessCache(withArgs)
}

// GetDebugStats returns the debug stats
func (p *Probe) GetDebugStats() map[string]interface{} {
	debug := map[string]interface{}{
		"start_time": p.startTime.String(),
	}
	// TODO(Will): add manager state
	return debug
}

// HandleActions executes the actions of a triggered rule
func (p *Probe) HandleActions(rule *rules.Rule, event eval.Event) {
	ctx := &eval.Context{
		Event: event.(*model.Event),
	}

	p.PlatformProbe.HandleActions(ctx, rule)
}

// AddEventHandler sets a probe event handler
func (p *Probe) AddEventHandler(eventType model.EventType, handler EventHandler) error {
	if eventType >= model.MaxAllEventType {
		return errors.New("unsupported event type")
	}

	p.eventHandlers[eventType] = append(p.eventHandlers[eventType], handler)

	return nil
}

// AddFullAccessEventHandler sets a probe event handler for the UnknownEventType which requires access to all the struct fields
func (p *Probe) AddFullAccessEventHandler(handler FullAccessEventHandler) error {
	p.fullAccessEventHandlers[model.UnknownEventType] = append(p.fullAccessEventHandlers[model.UnknownEventType], handler)

	return nil
}

// AddCustomEventHandler set the probe event handler
func (p *Probe) AddCustomEventHandler(eventType model.EventType, handler CustomEventHandler) error {
	if eventType >= model.MaxAllEventType {
		return errors.New("unsupported event type")
	}

	p.customEventHandlers[eventType] = append(p.customEventHandlers[eventType], handler)

	return nil
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

func traceEvent(fmt string, marshaller func() ([]byte, model.EventType, error)) {
	if !seclog.DefaultLogger.IsTracing() {
		return
	}

	eventJSON, eventType, err := marshaller()
	if err != nil {
		seclog.DefaultLogger.TraceTagf(eventType, fmt, err)
		return
	}

	seclog.DefaultLogger.TraceTagf(eventType, fmt, string(eventJSON))
}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *Probe) AddDiscarderPushedCallback(cb DiscarderPushedCallback) {
	p.PlatformProbe.AddDiscarderPushedCallback(cb)
}

// DispatchCustomEvent sends a custom event to the probe event handler
func (p *Probe) DispatchCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	traceEvent("Dispatching custom event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalCustomEvent(event)
		return eventJSON, event.GetEventType(), err
	})

	// send wildcard first
	for _, handler := range p.customEventHandlers[model.UnknownEventType] {
		handler.HandleCustomEvent(rule, event)
	}

	// send specific event
	for _, handler := range p.customEventHandlers[event.GetEventType()] {
		handler.HandleCustomEvent(rule, event)
	}
}

// StatsPollingInterval returns the stats polling interval
func (p *Probe) StatsPollingInterval() time.Duration {
	return p.Config.Probe.StatsPollingInterval
}

// GetEventTags returns the event tags
func (p *Probe) GetEventTags(containerID string) []string {
	return p.PlatformProbe.GetEventTags(containerID)
}

// GetService returns the service name from the process tree
func (p *Probe) GetService(ev *model.Event) string {
	if service := ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent); service != "" {
		return service
	}
	return p.Config.RuntimeSecurity.HostServiceName
}

// NewEvaluationSet returns a new evaluation set with rule sets tagged by the passed-in tag values for the "ruleset" tag key
func (p *Probe) NewEvaluationSet(eventTypeEnabled map[eval.EventType]bool, ruleSetTagValues []string) (*rules.EvaluationSet, error) {
	var ruleSetsToInclude []*rules.RuleSet
	for _, ruleSetTagValue := range ruleSetTagValues {
		ruleOpts, evalOpts := rules.NewEvalOpts(eventTypeEnabled)

		ruleOpts.WithLogger(seclog.DefaultLogger)
		ruleOpts.WithReservedRuleIDs(events.AllCustomRuleIDs())
		if ruleSetTagValue == rules.DefaultRuleSetTagValue {
			ruleOpts.WithSupportedDiscarders(SupportedDiscarders)
		}

		eventCtor := func() eval.Event {
			return p.PlatformProbe.NewEvent()
		}

		rs := rules.NewRuleSet(p.PlatformProbe.NewModel(), eventCtor, ruleOpts.WithRuleSetTag(ruleSetTagValue), evalOpts)
		ruleSetsToInclude = append(ruleSetsToInclude, rs)
	}

	evaluationSet, err := rules.NewEvaluationSet(ruleSetsToInclude)
	if err != nil {
		return nil, err
	}

	return evaluationSet, nil
}

// IsNetworkEnabled returns whether network is enabled
func (p *Probe) IsNetworkEnabled() bool {
	return p.Config.Probe.NetworkEnabled
}

// IsActivityDumpEnabled returns whether activity dump is enabled
func (p *Probe) IsActivityDumpEnabled() bool {
	return p.Config.RuntimeSecurity.ActivityDumpEnabled
}

// IsActivityDumpTagRulesEnabled returns whether rule tags is enabled for activity dumps
func (p *Probe) IsActivityDumpTagRulesEnabled() bool {
	return p.Config.RuntimeSecurity.ActivityDumpTagRulesEnabled
}

// IsSecurityProfileEnabled returns whether security profile is enabled
func (p *Probe) IsSecurityProfileEnabled() bool {
	return p.Config.RuntimeSecurity.SecurityProfileEnabled
}
