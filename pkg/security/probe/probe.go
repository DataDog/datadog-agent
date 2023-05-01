// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package probe

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// EventHandler represents an handler for the events sent by the probe
type EventHandler interface {
	HandleEvent(event *model.Event)
}

// CustomEventHandler represents an handler for the custom events sent by the probe
type CustomEventHandler interface {
	HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent)
}

// NotifyDiscarderPushedCallback describe the callback used to retrieve pushed discarders information
type NotifyDiscarderPushedCallback func(eventType string, event *model.Event, field string)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	PlatformProbe

	// Constants and configuration
	Opts         Opts
	Config       *config.Config
	StatsdClient statsd.ClientInterface
	startTime    time.Time
	ctx          context.Context
	cancelFnc    context.CancelFunc
	wg           sync.WaitGroup

	// Events section
	eventHandlers       [model.MaxAllEventType][]EventHandler
	customEventHandlers [model.MaxAllEventType][]CustomEventHandler

	discarderRateLimiter *rate.Limiter
	// internals
	resolvers     *resolvers.Resolvers
	fieldHandlers *FieldHandlers
	event         *model.Event
}

// GetResolvers returns the resolvers of Probe
func (p *Probe) GetResolvers() *resolvers.Resolvers {
	return p.resolvers
}

// AddEventHandler set the probe event handler
func (p *Probe) AddEventHandler(eventType model.EventType, handler EventHandler) error {
	if eventType >= model.MaxAllEventType {
		return errors.New("unsupported event type")
	}

	p.eventHandlers[eventType] = append(p.eventHandlers[eventType], handler)

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

func (p *Probe) zeroEvent() *model.Event {
	*p.event = eventZero
	p.event.FieldHandlers = p.fieldHandlers
	return p.event
}

func (p *Probe) StatsPollingInterval() time.Duration {
	return p.Config.Probe.StatsPollingInterval
}

// GetEventTags returns the event tags
func (p *Probe) GetEventTags(ev *model.Event) []string {
	return p.GetResolvers().TagsResolver.Resolve(ev.ContainerContext.ID)
}

// GetService returns the service name from the process tree
func (p *Probe) GetService(ev *model.Event) string {
	if service := ev.FieldHandlers.GetProcessService(ev); service != "" {
		return service
	}
	return p.Config.RuntimeSecurity.HostServiceName
}
