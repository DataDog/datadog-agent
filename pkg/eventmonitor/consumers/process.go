// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || windows

package consumers

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProcessCallback is a callback function that will be called when a process exec/exit event is received
// Defined as an alias to avoid type incompatibilities
type ProcessCallback = func(uint32)

// ProcessConsumer represents a consumer of process exec/exit events that can be subscribed to
// via callbacks
type ProcessConsumer struct {
	// id is the ID of the consumer
	id string

	// chanSize is the size of the channel that the event monitor will use to send events to this consumer
	chanSize int

	// eventTypes is the list of event types that this consumer is interested in
	eventTypes []ProcessConsumerEventTypes

	// execCallbacks holds all subscribers to process exec events
	execCallbacks *callbackMap

	// exitCallbacks holds all subscribers to process exit events
	exitCallbacks *callbackMap

	// forkCallbacks holds all subscribers to process fork events
	forkCallbacks *callbackMap
}

// event represents the attributes of the generic eventmonitor type that we will copy and use in our process consumer
type event struct {
	eventType model.EventType
	pid       uint32
}

// ProcessConsumerEventTypes represents the types of events that the ProcessConsumer can handle, a subset
// of the model.EventType
type ProcessConsumerEventTypes model.EventType

const (
	// ExecEventType represents process open events
	ExecEventType ProcessConsumerEventTypes = ProcessConsumerEventTypes(model.ExecEventType)

	// ExitEventType represents process exit events
	ExitEventType ProcessConsumerEventTypes = ProcessConsumerEventTypes(model.ExitEventType)

	// ForkEventType represents process fork events
	ForkEventType ProcessConsumerEventTypes = ProcessConsumerEventTypes(model.ForkEventType)
)

// ProcessConsumer should implement the EventConsumerHandler and EventConsumer interfaces
var _ eventmonitor.EventConsumerHandler = &ProcessConsumer{}
var _ eventmonitor.EventConsumer = &ProcessConsumer{}

// NewProcessConsumer creates a new ProcessConsumer, registering itself with the
// given event monitor. This function should be called with the EventMonitor
// instance created in cmd/system-probe/modules/eventmonitor.go:createEventMonitorModule.
// For tests, use consumers/testutil.NewTestProcessConsumer, which also initializes the event stream for testing accordingly
func NewProcessConsumer(id string, chanSize int, eventTypes []ProcessConsumerEventTypes, evm *eventmonitor.EventMonitor) (*ProcessConsumer, error) {
	pc := &ProcessConsumer{
		id:         id,
		chanSize:   chanSize,
		eventTypes: eventTypes,
	}

	if pc.isEventTypeEnabled(ExecEventType) {
		pc.execCallbacks = newCallbackMap()
	}

	if pc.isEventTypeEnabled(ExitEventType) {
		pc.exitCallbacks = newCallbackMap()
	}

	if pc.isEventTypeEnabled(ForkEventType) {
		pc.forkCallbacks = newCallbackMap()
	}

	if err := evm.AddEventConsumerHandler(pc); err != nil {
		return nil, fmt.Errorf("cannot add event consumer handler: %w", err)
	}

	evm.RegisterEventConsumer(pc)

	return pc, nil
}

func (p *ProcessConsumer) isEventTypeEnabled(eventType ProcessConsumerEventTypes) bool {
	return slices.Contains(p.eventTypes, eventType)
}

// --- eventmonitor.EventConsumer interface methods

// Start starts the consumer, this is a no-op for this implementation
func (p *ProcessConsumer) Start() error {
	return nil
}

// Stop stops the consumer, this is a no-op for this implementation
func (p *ProcessConsumer) Stop() {
}

// ID returns the ID of the consumer
func (p *ProcessConsumer) ID() string {
	return p.id
}

// --- eventmonitor.EventConsumerHandler interface methods

// ChanSize returns the size of the channel that the event monitor will use to send events to this handler
func (p *ProcessConsumer) ChanSize() int {
	return p.chanSize
}

// EventTypes returns the event types that this handler is interested in
func (p *ProcessConsumer) EventTypes() []model.EventType {
	var types []model.EventType
	for _, et := range p.eventTypes {
		types = append(types, model.EventType(et))
	}

	return types
}

// Copy copies the event from the eventmonitor type to the one we use in this struct
func (p *ProcessConsumer) Copy(ev *model.Event) any {
	return &event{
		eventType: ev.GetEventType(),
		pid:       ev.GetProcessPid(),
	}
}

// HandleEvent handles the event from the event monitor
func (p *ProcessConsumer) HandleEvent(ev any) {
	sevent, ok := ev.(*event)
	if !ok {
		return
	}

	switch sevent.eventType {
	case model.ExecEventType:
		p.execCallbacks.call(sevent.pid)
	case model.ExitEventType:
		p.exitCallbacks.call(sevent.pid)
	case model.ForkEventType:
		p.forkCallbacks.call(sevent.pid)
	}
}

// SubscribeExec subscribes to process exec events, and returns the function
// that needs to be called to unsubscribe
func (p *ProcessConsumer) SubscribeExec(callback ProcessCallback) func() {
	return p.execCallbacks.add(callback)
}

// SubscribeExit subscribes to process exit events, and returns the function
// that needs to be called to unsubscribe
func (p *ProcessConsumer) SubscribeExit(callback ProcessCallback) func() {
	return p.exitCallbacks.add(callback)
}

// SubscribeFork subscribes to process fork events, and returns the function
// that needs to be called to unsubscribe. Important: these callbacks will only be called if the
// ProcessConsumer was created with the ListenToForkEvents option set to true
func (p *ProcessConsumer) SubscribeFork(callback ProcessCallback) func() {
	return p.forkCallbacks.add(callback)
}

// callbackMap is a helper struct that holds a map of callbacks and a mutex to protect it
type callbackMap struct {
	// callbacks holds the set of callbacks
	callbacks map[*ProcessCallback]struct{}

	// mutex is the mutex that protects the callbacks map
	mutex sync.RWMutex

	// hasCallbacks is a flag that indicates if there are any callbacks subscribed, used
	// to avoid locking/unlocking the mutex if there are no callbacks
	hasCallbacks atomic.Bool
}

func newCallbackMap() *callbackMap {
	return &callbackMap{
		callbacks:    make(map[*ProcessCallback]struct{}),
		hasCallbacks: atomic.Bool{},
	}
}

// add adds a callback to the callback map and returns a function that can be called to remove it
func (c *callbackMap) add(cb ProcessCallback) func() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.callbacks[&cb] = struct{}{}
	c.hasCallbacks.Store(true)

	return func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		delete(c.callbacks, &cb)
		c.hasCallbacks.Store(len(c.callbacks) > 0)
	}
}

func (c *callbackMap) call(pid uint32) {
	if !c.hasCallbacks.Load() {
		return
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()
	for cb := range c.callbacks {
		(*cb)(pid)
	}
}
