// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package process

import (
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
)

// callbackMap is a helper struct that holds a map of callbacks and a mutex to protect it
type callbackMap struct {
	// callbacks holds the set of callbacks
	callbacks map[*consumers.ProcessCallback]struct{}

	// mutex is the mutex that protects the callbacks map
	mutex sync.RWMutex

	// hasCallbacks is a flag that indicates if there are any callbacks subscribed, used
	// to avoid locking/unlocking the mutex if there are no callbacks
	hasCallbacks atomic.Bool
}

func newCallbackMap() *callbackMap {
	return &callbackMap{
		callbacks: make(map[*consumers.ProcessCallback]struct{}),
	}
}

// add adds a callback to the callback map and returns a function that can be called to remove it
func (c *callbackMap) add(cb consumers.ProcessCallback) func() {
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
