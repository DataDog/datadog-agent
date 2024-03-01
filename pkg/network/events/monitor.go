// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package events handles process events
package events

import (
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var theMonitor atomic.Value
var once sync.Once
var initErr error

// Process is a process
type Process struct {
	Pid         uint32
	Envs        []string
	ContainerID *intern.Value
	StartTime   int64
	Expiry      int64
}

// Env returns the value of a environment variable
func (p *Process) Env(key string) string {
	for _, e := range p.Envs {
		k, v, _ := strings.Cut(e, "=")
		if k == key {
			return v
		}
	}

	return ""
}

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

// Initialized returns true if Init() has been called successfully
func Initialized() bool {
	return theMonitor.Load() != nil
}

//nolint:revive // TODO(NET) Fix revive linter
type ProcessEventHandler interface {
	HandleProcessEvent(*Process)
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

func (h *eventHandlerWrapper) HandleEvent(ev any) {
	if ev == nil {
		log.Errorf("Received nil event")
		return
	}

	evProcess, ok := ev.(*Process)
	if !ok {
		log.Errorf("Event is not a process")
		return
	}

	m := theMonitor.Load()
	if m != nil {
		m.(*eventMonitor).HandleEvent(evProcess)
	}
}

// Copy copies the necessary fields from the event received from the event monitor
func (h *eventHandlerWrapper) Copy(ev *model.Event) any {
	if theMonitor.Load() == nil {
		// monitor is not initialized, so need to copy the event
		// since it will get dropped by the handler anyway
		return nil
	}

	// If this consumer subscribes to more event types, this block will have to account for those additional event types
	var processStartTime time.Time
	if ev.GetEventType() == model.ExecEventType {
		processStartTime = ev.GetProcessExecTime()
	}
	if ev.GetEventType() == model.ForkEventType {
		processStartTime = ev.GetProcessForkTime()
	}

	envs := ev.GetProcessEnvp()

	return &Process{
		Pid:         ev.GetProcessPid(),
		ContainerID: intern.GetByString(ev.GetContainerId()),
		StartTime:   processStartTime.UnixNano(),
		Envs: model.FilterEnvs(envs, map[string]bool{
			"DD_SERVICE": true,
			"DD_VERSION": true,
			"DD_ENV":     true,
		}),
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

	handlers []ProcessEventHandler
}

func newEventMonitor() (*eventMonitor, error) {
	return &eventMonitor{}, nil
}

func (e *eventMonitor) HandleEvent(ev *Process) {
	e.Lock()
	defer e.Unlock()

	for _, h := range e.handlers {
		h.HandleProcessEvent(ev)
	}
}

//nolint:revive // TODO(NET) Fix revive linter
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
