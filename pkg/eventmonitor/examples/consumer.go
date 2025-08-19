// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/event_copy -scope "(fc *SimpleEventConsumer)" -pkg examples -output ./event_copy.go SimpleEvent .

// Package examples provides an example of how to use the event monitor module using a basic consumer
package examples

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SimpleEvent defines a simple event
type SimpleEvent struct {
	Type         uint32   `copy:"GetEventType;event:*;cast:uint32"`
	ExecFilePath string   `copy:"GetExecFilePath;event:ExecEventType"`
	Envp         []string `copy:"GetProcessEnvp;event:ExecEventType"`
}

// SimpleEventConsumer defines a simple event consumer
type SimpleEventConsumer struct {
	sync.RWMutex
	exec int
	fork int
	exit int

	handlers []func(evt *SimpleEvent)
}

// NewSimpleEventConsumer returns a new simple event consumer
func NewSimpleEventConsumer(em *eventmonitor.EventMonitor) *SimpleEventConsumer {
	fc := &SimpleEventConsumer{}
	_ = em.AddEventConsumerHandler(fc)
	return fc
}

// AddHandler adds a handler to this consumer
func (fc *SimpleEventConsumer) AddHandler(handler func(evt *SimpleEvent)) {
	fc.Lock()
	defer fc.Unlock()

	fc.handlers = append(fc.handlers, handler)
}

// ID returns the ID of this consumer
// Implement the consumer interface
func (fc *SimpleEventConsumer) ID() string {
	return "simple_consumer"
}

// Start the consumer
// Implement the consumer interface
func (fc *SimpleEventConsumer) Start() error {
	return nil
}

// Stop the consumer
// Implement the consumer interface
func (fc *SimpleEventConsumer) Stop() {
}

// EventTypes returns the event types handled by this consumer
// Implement the consumer interface
func (fc *SimpleEventConsumer) EventTypes() []model.EventType {
	return []model.EventType{
		model.ForkEventType,
		model.ExecEventType,
		model.ExitEventType,
	}
}

// ChanSize returns the chan size used by the consumer
func (fc *SimpleEventConsumer) ChanSize() int {
	return 50
}

// HandleEvent handles this event
// Implement the consumer interface
func (fc *SimpleEventConsumer) HandleEvent(event any) {
	sevent, ok := event.(*SimpleEvent)
	if !ok {
		log.Error("Event is not a security model event")
		return
	}

	fc.Lock()
	defer fc.Unlock()

	switch sevent.Type {
	case uint32(model.ExecEventType):
		fc.exec++
	case uint32(model.ForkEventType):
		fc.fork++
	case uint32(model.ExitEventType):
		fc.exit++
	}

	for _, handler := range fc.handlers {
		handler(sevent)
	}
}

// ForkCount returns the number of fork handled
func (fc *SimpleEventConsumer) ForkCount() int {
	fc.RLock()
	defer fc.RUnlock()
	return fc.fork
}

// ExitCount returns the number of exit handled
func (fc *SimpleEventConsumer) ExitCount() int {
	fc.RLock()
	defer fc.RUnlock()
	return fc.exit
}

// ExecCount returns the number of exec handled
func (fc *SimpleEventConsumer) ExecCount() int {
	fc.RLock()
	defer fc.RUnlock()
	return fc.exec
}
