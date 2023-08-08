// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package events

import (
	"log"
	"sync"

	"go.uber.org/atomic"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var theMonitor atomic.Value
var once sync.Once
var initErr error

// Init initializes the events package
func Init() error {
	once.Do(func() {
		var m *eventMonitor
		m, initErr = newEventMonitor()
		if initErr == nil {
			theMonitor.Store(m)
		}
	})

	return initErr
}

type ProcessEventHandler interface {
	HandleProcessEvent(*model.ProcessContext)
}

// RegisterHandler registers a handler function for getting process events
func RegisterHandler(handler ProcessEventHandler) {
	m := theMonitor.Load().(*eventMonitor)
	m.RegisterHandler(handler)
}

// UnregisterHandler unregisters a handler function for getting process events
func UnregisterHandler(handler ProcessEventHandler) {
	m := theMonitor.Load().(*eventMonitor)
	m.UnregisterHandler(handler)
}

type eventHandlerWrapper struct{}

func (h *eventHandlerWrapper) HandleEvent(ev model.EventInterface) {
	m := theMonitor.Load()
	if m != nil {
		m.(*eventMonitor).HandleEvent(ev)
	}
}

// IsEventMonitorConsumer returns if the Event Handler is an Event Monitor Consumer
func (h *eventHandlerWrapper) IsEventMonitorConsumer() bool {
	return true
}

func (h *eventHandlerWrapper) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	m := theMonitor.Load()
	if m != nil {
		m.(*eventMonitor).HandleCustomEvent(rule, event)
	}
}

var _eventHandlerWrapper = &eventHandlerWrapper{}

// Handler returns an event handler to handle events from the runtime security module
func Handler() sprobe.EventHandler {
	return _eventHandlerWrapper
}

type eventMonitor struct {
	sync.Mutex

	handlers []ProcessEventHandler
}

func newEventMonitor() (*eventMonitor, error) {
	return &eventMonitor{}, nil
}

func (e *eventMonitor) HandleEvent(incomingEvent model.EventInterface) {
	//ev.ResolveFields()

	e.Lock()
	defer e.Unlock()

	// TODO: This is actually a model.ProcessEvent, NOT model.Event. h(ev.ProcessContext) needs to be changed
	ev, ok := incomingEvent.(*model.Event)

	if !ok {
		log.Fatal("Consumer received unknown object")
	}

	for _, h := range e.handlers {
		h.HandleProcessEvent(ev.ProcessContext)
	}
}

// IsEventMonitorConsumer returns if the Event Handler is an Event Monitor Consumer
func (e *eventMonitor) IsEventMonitorConsumer() bool {
	return true
}

func (e *eventMonitor) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
}

func (e *eventMonitor) RegisterHandler(handler ProcessEventHandler) {
	if handler == nil {
		return
	}

	e.Lock()
	defer e.Unlock()

	e.handlers = append(e.handlers, handler)
}

func (e *eventMonitor) UnregisterHandler(handler ProcessEventHandler) {
	if handler == nil {
		return
	}

	e.Lock()
	defer e.Unlock()

	if idx := slices.Index(e.handlers, handler); idx >= 0 {
		e.handlers = slices.Delete(e.handlers, idx, idx+1)
	}
}
