// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

// boundedManagerSignalHandler delivers signals synchronously and never queues
// work outside the caller-provided bounded channels. An overflow is terminal
// for a manager job wait because a dropped JobRemoved signal makes the final
// mutation outcome unknowable.
type boundedManagerSignalHandler struct {
	mu           sync.Mutex
	closed       bool
	channels     []chan<- *dbus.Signal
	overflow     chan struct{}
	overflowOnce sync.Once
}

func newBoundedManagerSignalHandler() *boundedManagerSignalHandler {
	return &boundedManagerSignalHandler{overflow: make(chan struct{})}
}

func (h *boundedManagerSignalHandler) DeliverSignal(_, _ string, signal *dbus.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for _, channel := range h.channels {
		select {
		case channel <- signal:
		default:
			h.overflowOnce.Do(func() { close(h.overflow) })
		}
	}
}

func (h *boundedManagerSignalHandler) AddSignal(channel chan<- *dbus.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for _, registered := range h.channels {
		if registered == channel {
			return
		}
	}
	h.channels = append(h.channels, channel)
}

func (h *boundedManagerSignalHandler) RemoveSignal(channel chan<- *dbus.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for index := len(h.channels) - 1; index >= 0; index-- {
		if h.channels[index] == channel {
			copy(h.channels[index:], h.channels[index+1:])
			h.channels[len(h.channels)-1] = nil
			h.channels = h.channels[:len(h.channels)-1]
		}
	}
}

func (h *boundedManagerSignalHandler) Terminate() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for _, channel := range h.channels {
		close(channel)
	}
	h.channels = nil
	h.closed = true
}

func (h *boundedManagerSignalHandler) Overflow() <-chan struct{} {
	return h.overflow
}
