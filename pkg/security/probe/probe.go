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
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// FullAccessEventHandler represents a handler for events sent by the probe that needs access to all the fields in the SECL model
type FullAccessEventHandler interface {
	HandleEvent(event *model.Event)
}

// EventHandler represents a handler for events sent by the probe. This handler makes a copy of the event upon receipt
type EventHandler[T any] interface {
	HandleEvent(event T)
	Copy(_ *model.Event) T
}

// CustomEventHandler represents an handler for the custom events sent by the probe
type CustomEventHandler interface {
	HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent)
}

// NotifyDiscarderPushedCallback describe the callback used to retrieve pushed discarders information
type NotifyDiscarderPushedCallback func(eventType string, event *model.Event, field string)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe[T any] struct {
	PlatformProbe

	// Constants and configuration
	Opts         Opts
	Config       *config.Config
	StatsdClient statsd.ClientInterface
	startTime    time.Time
	ctx          context.Context
	cancelFnc    context.CancelFunc
	wg           sync.WaitGroup

	// internals
	scrubber *procutil.DataScrubber

	// Events section
	fullAccessEventHandlers [model.MaxAllEventType][]FullAccessEventHandler
	eventHandlers           [model.MaxAllEventType][]EventHandler[T]
	customEventHandlers     [model.MaxAllEventType][]CustomEventHandler

	discarderRateLimiter *rate.Limiter
	// internals
	resolvers     *resolvers.Resolvers
	fieldHandlers *FieldHandlers
	event         *model.Event
}

// GetResolvers returns the resolvers of Probe
func (p *Probe[T]) GetResolvers() *resolvers.Resolvers {
	return p.resolvers
}

// AddEventHandler sets a probe event handler
func (p *Probe[T]) AddEventHandler(eventType model.EventType, handler EventHandler[T]) error {
	if eventType >= model.MaxAllEventType {
		return errors.New("unsupported event type")
	}

	p.eventHandlers[eventType] = append(p.eventHandlers[eventType], handler)

	return nil
}

// AddFullAccessEventHandler sets a probe event handler for the UnknownEventType which requires access to all the struct fields
func (p *Probe[T]) AddFullAccessEventHandler(handler FullAccessEventHandler) error {
	p.fullAccessEventHandlers[model.UnknownEventType] = append(p.fullAccessEventHandlers[model.UnknownEventType], handler)

	return nil
}

// AddCustomEventHandler set the probe event handler
func (p *Probe[T]) AddCustomEventHandler(eventType model.EventType, handler CustomEventHandler) error {
	if eventType >= model.MaxAllEventType {
		return errors.New("unsupported event type")
	}

	p.customEventHandlers[eventType] = append(p.customEventHandlers[eventType], handler)

	return nil
}

func (p *Probe[T]) zeroEvent() *model.Event {
	p.event.Zero()
	p.event.FieldHandlers = p.fieldHandlers
	return p.event
}

// StatsPollingInterval returns the stats polling interval
func (p *Probe[T]) StatsPollingInterval() time.Duration {
	return p.Config.Probe.StatsPollingInterval
}

// GetEventTags returns the event tags
func (p *Probe[T]) GetEventTags(containerID string) []string {
	return p.GetResolvers().TagsResolver.Resolve(containerID)
}

// GetService returns the service name from the process tree
func (p *Probe[T]) GetService(ev *model.Event) string {
	if service := ev.FieldHandlers.GetProcessService(ev); service != "" {
		return service
	}
	return p.Config.RuntimeSecurity.HostServiceName
}

// NewEvaluationSet returns a new evaluation set with rule sets tagged by the passed-in tag values for the "ruleset" tag key
func (p *Probe[T]) NewEvaluationSet(eventTypeEnabled map[eval.EventType]bool, ruleSetTagValues []string) (*rules.EvaluationSet, error) {
	var ruleSetsToInclude []*rules.RuleSet
	for _, ruleSetTagValue := range ruleSetTagValues {
		ruleOpts, evalOpts := rules.NewEvalOpts(eventTypeEnabled)

		ruleOpts.WithLogger(seclog.DefaultLogger)
		ruleOpts.WithReservedRuleIDs(events.AllCustomRuleIDs())
		if ruleSetTagValue == rules.DefaultRuleSetTagValue {
			ruleOpts.WithSupportedDiscarders(SupportedDiscarders)
		}

		eventCtor := func() eval.Event {
			return NewEvent(p.fieldHandlers)
		}

		rs := rules.NewRuleSet(NewModel(p), eventCtor, ruleOpts.WithRuleSetTag(ruleSetTagValue), evalOpts)
		ruleSetsToInclude = append(ruleSetsToInclude, rs)
	}

	evaluationSet, err := rules.NewEvaluationSet(ruleSetsToInclude)
	if err != nil {
		return nil, err
	}

	return evaluationSet, nil
}

// IsNetworkEnabled returns whether network is enabled
func (p *Probe[T]) IsNetworkEnabled() bool {
	return p.Config.Probe.NetworkEnabled
}

// IsActivityDumpEnabled returns whether activity dump is enabled
func (p *Probe[T]) IsActivityDumpEnabled() bool {
	return p.Config.RuntimeSecurity.ActivityDumpEnabled
}

// IsActivityDumpTagRulesEnabled returns whether rule tags is enabled for activity dumps
func (p *Probe[T]) IsActivityDumpTagRulesEnabled() bool {
	return p.Config.RuntimeSecurity.ActivityDumpTagRulesEnabled
}

// IsSecurityProfileEnabled returns whether security profile is enabled
func (p *Probe[T]) IsSecurityProfileEnabled() bool {
	return p.Config.RuntimeSecurity.SecurityProfileEnabled
}
