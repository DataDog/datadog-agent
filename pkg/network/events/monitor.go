// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/event_copy -scope "(h *eventConsumerWrapper)" -pkg events -output event_copy_linux.go Process .

//go:build linux || windows

// Package events handles process events
package events

import (
	"slices"
	"strings"
	"sync"

	"go.uber.org/atomic"
	"go4.org/intern"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	chanSize = 100
)

var (
	theMonitor atomic.Value
	once       sync.Once
	initErr    error
	envFilter  = map[string]bool{
		"DD_SERVICE": true,
		"DD_VERSION": true,
		"DD_ENV":     true,
	}
	envTagNames = map[string]string{
		"DD_SERVICE": "service",
		"DD_VERSION": "version",
		"DD_ENV":     "env",
	}
)

// Process is a process
type Process struct {
	Pid         uint32
	Tags        []*intern.Value
	ContainerID *intern.Value
	StartTime   int64
	Expiry      int64
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

// ProcessEventHandler represents a handler function triggered for process events
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

type eventConsumerWrapper struct{}

func (h *eventConsumerWrapper) HandleEvent(ev any) {
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
func (h *eventConsumerWrapper) Copy(ev *model.Event) any {
	if theMonitor.Load() == nil {
		return nil
	}

	// If this consumer subscribes to more event types, this block will have to account for those additional event types
	processStartTime := getProcessStartTime(ev)

	p := &Process{
		Pid:       ev.GetProcessPid(),
		StartTime: processStartTime.UnixNano(),
	}

	// we need to keep looking for settings until all of the desired
	// key/value pairs are found, following the precedence order.

	// the precedence order is
	// 1. environment variables
	// 2. tags from the web.config file
	// 3. tags from the datadog.json file

	// keep the map of tagsFound, so that entries found at lower levels
	// don't supercede those at higher.  However, we must actually parse
	// all of the inputs to ensure that we don't miss any tags that are
	tagsFound := make(map[string]struct{})

	envs := model.FilterEnvs(ev.GetProcessEnvp(), envFilter)
	if len(envs) > 0 {
		p.Tags = make([]*intern.Value, 0, len(envs))
		for _, env := range envs {
			k, v, _ := strings.Cut(env, "=")
			if len(v) > 0 {
				if t := envTagNames[k]; t != "" {
					p.Tags = append(p.Tags, intern.GetByString(t+":"+v))
					tagsFound[k] = struct{}{}
				}
			}
		}
	}

	if len(tagsFound) < len(envFilter) {
		apmTags := getAPMTags(tagsFound, ev.GetExecFilePath())
		if len(apmTags) > 0 {
			p.Tags = append(p.Tags, apmTags...)
		}
	}

	if cid := ev.GetContainerId(); cid != "" {
		p.ContainerID = intern.GetByString(cid)
	}

	return p
}

// EventTypes returns the event types handled by this consumer
func (h *eventConsumerWrapper) EventTypes() []model.EventType {
	return []model.EventType{
		model.ForkEventType,
		model.ExecEventType,
	}
}

// ChanSize returns the chan size used by this consumer
func (h *eventConsumerWrapper) ChanSize() int {
	return chanSize
}

// ID returns the id of this consumer
func (h eventConsumerWrapper) ID() string {
	return "network"
}

var _eventConsumerWrapper = &eventConsumerWrapper{}

// Consumer returns an event consumer to handle events from the runtime security module
func Consumer() sprobe.EventConsumerHandler {
	return _eventConsumerWrapper
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
