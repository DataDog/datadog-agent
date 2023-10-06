// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package windowsevent

import "sync"

// eventContextToTailerMap allows a map between event contexts and tailers.
// When we subscribe to event logs, we need to know in our callback
// function which tailer this event is for, so we apply right tags
// and send it to the proper output channel.
// Problem is when we pass an eventContext that will be shared back
// to the golang callback, it cannot contain pointers
// (panic: runtime error: cgo argument has Go pointer to Go pointer)
// As a result, we need to keep a global map of index -> tailer
var eventContextToTailerMap = make(map[int]*Tailer)
var lock = sync.RWMutex{}

// tailerForIndex returns the tailer which maps to the specified index
func tailerForIndex(id int) (*Tailer, bool) {
	lock.RLock()
	defer lock.RUnlock()
	tailer, exists := eventContextToTailerMap[id]
	return tailer, exists
}

// indexForTailer adds a tailer in the global map, and returs its index
func indexForTailer(t *Tailer) int {
	lock.Lock()
	defer lock.Unlock()
	nextID := len(eventContextToTailerMap)
	eventContextToTailerMap[nextID] = t
	return nextID
}
