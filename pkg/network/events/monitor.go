// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package events

import (
	"sync"

	"go.uber.org/atomic"

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

// HandlerFunc is the prototype for an event handler callback for process events
type HandlerFunc func(*model.ProcessCacheEntry)

// RegisterHandler registers a handler function for getting process events
func RegisterHandler(handler HandlerFunc) {
	m := theMonitor.Load().(*eventMonitor)
	m.RegisterHandler(handler)
}

type eventHandlerWrapper struct{}

func (h *eventHandlerWrapper) HandleEvent(ev *model.Event) {
	m := theMonitor.Load()
	if m != nil {
		m.(*eventMonitor).HandleEvent(ev)
	}
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

	handlers []HandlerFunc
}

func newEventMonitor() (*eventMonitor, error) {
	return &eventMonitor{}, nil
}

func (e *eventMonitor) HandleEvent(ev *model.Event) {
	if ev.Type == uint32(model.ExitEventType) {
		return
	}

	_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.ProcessContext.Process)

	entry, ok := ev.ResolveProcessCacheEntry()
	if !ok {
		return
	}

	e.Lock()
	defer e.Unlock()

	for _, h := range e.handlers {
		h(entry)
	}

}

func (e *eventMonitor) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
}

func (e *eventMonitor) RegisterHandler(handler HandlerFunc) {
	if handler == nil {
		return
	}

	e.Lock()
	defer e.Unlock()

	e.handlers = append(e.handlers, handler)

}
