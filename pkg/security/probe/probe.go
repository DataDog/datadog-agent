// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	defaultConsumerChanSize = 50
)

// PlatformProbe defines a platform dependent probe
type PlatformProbe interface {
	Init() error
	Start() error
	Stop()
	SendStats() error
	Snapshot() error
	Walk(_ func(_ *model.ProcessCacheEntry))
	Close() error
	NewModel() *model.Model
	DumpDiscarders() (string, error)
	FlushDiscarders() error
	ApplyRuleSet(_ *rules.RuleSet) (*kfilters.FilterReport, bool, error)
	OnNewRuleSetLoaded(_ *rules.RuleSet)
	OnNewDiscarder(_ *rules.RuleSet, _ *model.Event, _ eval.Field, _ eval.EventType)
	HandleActions(_ *eval.Context, _ *rules.Rule)
	NewEvent() *model.Event
	GetFieldHandlers() model.FieldHandlers
	DumpProcessCache(_ bool) (string, error)
	AddDiscarderPushedCallback(_ DiscarderPushedCallback)
	GetEventTags(_ containerutils.ContainerID) []string
	EnableEnforcement(bool)
	ReplayEvents()
}

var probeTelemetry = struct {
	totalVariables telemetry.Gauge
}{
	totalVariables: metrics.NewITGauge(metrics.MetricSECLTotalVariables, []string{"type", "scope"}, "Number of instantiated variables"),
}

var probeEventZeroer = model.NewEventZeroer()

// EventConsumer defines a probe event consumer
type EventConsumer struct {
	consumer     EventConsumerHandler
	eventCh      chan any
	eventDropped *atomic.Int64
}

// Start the consumer
func (p *EventConsumer) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case event := <-p.eventCh:
				p.consumer.HandleEvent(event)
			}
		}
	}()
}

// DiscarderPushedCallback describe the callback used to retrieve pushed discarders information
type DiscarderPushedCallback func(eventType string, event *model.Event, field string)

type actionStatsTags struct {
	ruleID     rules.RuleID
	actionName rules.ActionName
}

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	PlatformProbe         PlatformProbe
	agentContainerContext *events.AgentContainerContext

	// Constants and configuration
	Opts         Opts
	Config       *config.Config
	StatsdClient statsd.ClientInterface

	// internals
	ctx       context.Context
	cancelFnc func()
	wg        sync.WaitGroup
	startTime time.Time
	scrubber  *utils.Scrubber

	// Events section
	consumers           []*EventConsumer
	eventHandlers       [model.MaxAllEventType][]EventHandler
	eventConsumers      [model.MaxAllEventType][]*EventConsumer
	customEventHandlers [model.MaxAllEventType][]CustomEventHandler

	// stats
	ruleActionStatsLock sync.RWMutex
	ruleActionStats     map[actionStatsTags]*atomic.Int64
}

func newProbe(config *config.Config, opts Opts) (*Probe, error) {
	scrubber, err := utils.NewScrubber(config.Probe.CustomSensitiveWords, config.Probe.CustomSensitiveRegexps)
	if err != nil {
		return nil, fmt.Errorf("failed to create event scrubber: %w", err)
	}

	return &Probe{
		Opts:            opts,
		Config:          config,
		StatsdClient:    opts.StatsdClient,
		scrubber:        scrubber,
		ruleActionStats: make(map[actionStatsTags]*atomic.Int64),
	}, nil
}

// Init initializes the probe
func (p *Probe) Init() error {
	p.startTime = time.Now()
	return p.PlatformProbe.Init()
}

// Start plays the snapshot data and then start the event stream
func (p *Probe) Start() error {
	p.ctx, p.cancelFnc = context.WithCancel(context.Background())

	for _, pc := range p.consumers {
		pc.Start(p.ctx, &p.wg)
	}

	return p.PlatformProbe.Start()
}

func (p *Probe) sendConsumerStats() error {
	for _, consumer := range p.consumers {
		dropped := consumer.eventDropped.Swap(0)
		if dropped > 0 {
			tags := []string{
				"consumer_id:" + consumer.consumer.ID(),
			}
			if err := p.StatsdClient.Count(metrics.MetricEventMonitoringEventsDropped, dropped, tags, 1.0); err != nil {
				return err
			}
		}
	}

	return nil
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	if err := p.sendConsumerStats(); err != nil {
		return err
	}

	p.ruleActionStatsLock.RLock()
	for tags, counter := range p.ruleActionStats {
		count := counter.Swap(0)
		if count > 0 {
			tags := []string{
				"rule_id:" + tags.ruleID,
				"action_name:" + tags.actionName,
			}
			_ = p.StatsdClient.Count(metrics.MetricRuleActionPerformed, count, tags, 1.0)
		}
	}
	p.ruleActionStatsLock.RUnlock()

	return p.PlatformProbe.SendStats()
}

// Close the probe
func (p *Probe) Close() error {
	return p.PlatformProbe.Close()
}

// Stop the probe
func (p *Probe) Stop() {
	if p.cancelFnc != nil {
		p.cancelFnc()
	}
	p.wg.Wait()

	p.PlatformProbe.Stop()
}

// FlushDiscarders invalidates all the discarders
func (p *Probe) FlushDiscarders() error {
	seclog.Debugf("Flushing discarders")
	return p.PlatformProbe.FlushDiscarders()
}

// ApplyRuleSet setup the probes for the provided set of rules and returns the policy report.
func (p *Probe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.FilterReport, bool, error) {
	return p.PlatformProbe.ApplyRuleSet(rs)
}

// OnNewRuleSetLoaded resets statistics and states once a new rule set is loaded
func (p *Probe) OnNewRuleSetLoaded(rs *rules.RuleSet) {
	p.ruleActionStatsLock.Lock()
	clear(p.ruleActionStats)
	p.ruleActionStatsLock.Unlock()
	p.PlatformProbe.OnNewRuleSetLoaded(rs)
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return p.PlatformProbe.Snapshot()
}

// Walk iterates through the entire tree and call the provided callback on each entry
func (p *Probe) Walk(cb func(entry *model.ProcessCacheEntry)) {
	p.PlatformProbe.Walk(cb)
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
	ctx := eval.NewContext(event.(*model.Event))

	p.PlatformProbe.HandleActions(ctx, rule)
}

// AddEventConsumer sets a probe event consumer
func (p *Probe) AddEventConsumer(consumer EventConsumerHandler) error {
	chanSize := consumer.ChanSize()
	if chanSize <= 0 {
		chanSize = defaultConsumerChanSize
	}

	pc := &EventConsumer{
		consumer:     consumer,
		eventCh:      make(chan any, chanSize),
		eventDropped: atomic.NewInt64(0),
	}

	for _, eventType := range consumer.EventTypes() {
		if eventType >= model.MaxAllEventType {
			return fmt.Errorf("event type (%s) not allowed", eventType)
		}

		p.eventConsumers[eventType] = append(p.eventConsumers[eventType], pc)
	}

	p.consumers = append(p.consumers, pc)

	return nil
}

// AddEventHandler sets a probe event handler for the UnknownEventType which requires access to all the struct fields
func (p *Probe) AddEventHandler(handler EventHandler) error {
	p.eventHandlers[model.UnknownEventType] = append(p.eventHandlers[model.UnknownEventType], handler)

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

func (p *Probe) sendEventToHandlers(event *model.Event) {
	for _, handler := range p.eventHandlers[model.UnknownEventType] {
		handler.HandleEvent(event)
	}
}

func (p *Probe) sendEventToConsumers(event *model.Event) {
	for _, pc := range p.eventConsumers[event.GetEventType()] {
		if copied := pc.consumer.Copy(event); copied != nil {
			select {
			case pc.eventCh <- copied:
			default:
				pc.eventDropped.Inc()
			}
		}
	}
}

func (p *Probe) logTraceEvent(eventType model.EventType, event interface{}) {
	if !seclog.DefaultLogger.IsTracing() {
		return
	}

	seclog.DefaultLogger.TraceTagf(eventType, "Dispatching event %s", serializers.EventStringerWrapper{Event: event, Scrubber: p.scrubber})
}

// GetScrubber returns the event scrubber
func (p *Probe) GetScrubber() *utils.Scrubber {
	return p.scrubber
}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *Probe) AddDiscarderPushedCallback(cb DiscarderPushedCallback) {
	p.PlatformProbe.AddDiscarderPushedCallback(cb)
}

// DispatchCustomEvent sends a custom event to the probe event handler
func (p *Probe) DispatchCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	p.logTraceEvent(event.GetEventType(), event)

	// send wildcard first
	for _, handler := range p.customEventHandlers[model.UnknownEventType] {
		handler.HandleCustomEvent(rule, event)
	}

	// send specific event
	if event.GetEventType() != model.UnknownEventType {
		for _, handler := range p.customEventHandlers[event.GetEventType()] {
			handler.HandleCustomEvent(rule, event)
		}
	}
}

// StatsPollingInterval returns the stats polling interval
func (p *Probe) StatsPollingInterval() time.Duration {
	return p.Config.Probe.StatsPollingInterval
}

// GetEventTags returns the event tags
func (p *Probe) GetEventTags(containerID containerutils.ContainerID) []string {
	return p.PlatformProbe.GetEventTags(containerID)
}

// GetService returns the service name from the process tree
func (p *Probe) GetService(ev *model.Event) string {
	if service := ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent); service != "" {
		return service
	}
	return p.Config.RuntimeSecurity.HostServiceName
}

func (p *Probe) onRuleActionPerformed(rule *rules.Rule, action *rules.ActionDefinition) {
	p.ruleActionStatsLock.Lock()
	defer p.ruleActionStatsLock.Unlock()

	tags := actionStatsTags{
		ruleID:     rule.ID,
		actionName: action.Name(),
	}

	var counter *atomic.Int64
	if counter = p.ruleActionStats[tags]; counter == nil {
		counter = atomic.NewInt64(1)
		p.ruleActionStats[tags] = counter
	} else {
		counter.Inc()
	}
}

// ReplayEvents replays the events from the rule set
func (p *Probe) ReplayEvents() {
	p.PlatformProbe.ReplayEvents()
}

// NewRuleSet returns a new ruleset
func (p *Probe) NewRuleSet(eventTypeEnabled map[eval.EventType]bool) *rules.RuleSet {
	ruleOpts, evalOpts := rules.NewBothOpts(eventTypeEnabled)
	ruleOpts.WithLogger(seclog.DefaultLogger)
	ruleOpts.WithReservedRuleIDs(events.AllCustomRuleIDs())
	ruleOpts.WithSupportedDiscarders(SupportedDiscarders)
	ruleOpts.WithSupportedMultiDiscarder(SupportedMultiDiscarder)
	ruleOpts.WithRuleActionPerformedCb(p.onRuleActionPerformed)
	ruleOpts.WithRuleCacheEnabled(p.Config.RuntimeSecurity.RuleCacheEnabled)
	evalOpts.WithTelemetry(&eval.Telemetry{TotalVariables: probeTelemetry.totalVariables})

	eventCtor := func() eval.Event {
		return p.PlatformProbe.NewEvent()
	}

	return rules.NewRuleSet(p.PlatformProbe.NewModel(), eventCtor, ruleOpts, evalOpts)
}

// IsNetworkEnabled returns whether network is enabled
func (p *Probe) IsNetworkEnabled() bool {
	return p.Config.Probe.NetworkEnabled
}

// IsNetworkRawPacketEnabled returns whether network raw packet is enabled
func (p *Probe) IsNetworkRawPacketEnabled() bool {
	return p.IsNetworkEnabled() && p.Config.Probe.NetworkRawPacketEnabled
}

// IsNetworkFlowMonitorEnabled returns whether the network flow monitor is enabled
func (p *Probe) IsNetworkFlowMonitorEnabled() bool {
	return p.IsNetworkEnabled() && p.Config.Probe.NetworkFlowMonitorEnabled
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

// EnableEnforcement sets the enforcement mode
func (p *Probe) EnableEnforcement(state bool) {
	p.PlatformProbe.EnableEnforcement(state)
}

// GetAgentContainerContext returns the agent container context
func (p *Probe) GetAgentContainerContext() *events.AgentContainerContext {
	return p.agentContainerContext
}
