// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"fmt"
	"sync"
)

type event interface{ comparable }

// Listener describes the callback called by a notifier
type Listener[O any] func(obj O)

// Notifier describes a type that calls back listener that registered for a specific set of events
type Notifier[E event, O any] struct {
	listenersLock sync.RWMutex
	listeners     map[E][]Listener[O]
}

// RegisterListener registers an event listener
func (n *Notifier[E, O]) RegisterListener(event E, listener Listener[O]) error {
	n.listenersLock.Lock()
	defer n.listenersLock.Unlock()

	if n.listeners != nil {
		n.listeners[event] = append(n.listeners[event], listener)
	} else {
		return fmt.Errorf("a listener was inserted before initialization")
	}
	return nil
}

// NotifyListeners notifies all listeners of an event type
func (n *Notifier[E, O]) NotifyListeners(event E, obj O) {
	// notify listeners
	n.listenersLock.RLock()
	for _, l := range n.listeners[event] {
		l(obj)
	}
	n.listenersLock.RUnlock()

}

// NewNotifier returns a new notifier
func NewNotifier[E event, O any]() *Notifier[E, O] {
	return &Notifier[E, O]{
		listeners: make(map[E][]Listener[O]),
	}
}
