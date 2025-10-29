// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"cmp"
	"sync"
	"time"

	"github.com/google/btree"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO: At some point we may want to add some fairness between different
// probes in the same process and between different processes. As currently
// implemented, if one process opens a bunch of functions that stay open for a
// long time, it will starve other processes of buffer space.

// Arbitrary degree for the btree.
//
// TODO: empirically determine the optimal degree.
const degree = 16

// Arbitrary free list size. This will be some constant overhead and then
// some factor of the degree.
const freeListSize = 16

// bufferedMessageTracker is a runtime-wide data structure tracking the memory
// usage of entry events waiting to be paired with their corresponding return
// events.
//
// Each program corresponding sink will hold a child bufferTree that updates
// this tracker when events are added or removed.
type bufferedMessageTracker struct {
	freelist  *btree.FreeListG[bufferedEvent]
	byteLimit int
	mu        struct {
		sync.Mutex
		used int
	}
}

func newBufferedMessageTracker(byteLimit int) *bufferedMessageTracker {
	return &bufferedMessageTracker{
		freelist:  btree.NewFreeListG[bufferedEvent](freeListSize),
		byteLimit: byteLimit,
	}
}

func (mb *bufferedMessageTracker) newTree() *bufferTree {
	return &bufferTree{
		tree: btree.NewWithFreeListG(degree, bufferedEventLess, mb.freelist),
		mb:   mb,
	}
}

func (mb *bufferedMessageTracker) add(size int) (ok bool) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.mu.used+size > mb.byteLimit {
		return false
	}
	mb.mu.used += size
	return true
}

func (mb *bufferedMessageTracker) release(size int) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.mu.used < size {
		log.Errorf("invariant violation: used %d < size %d", mb.mu.used, size)
		mb.mu.used = 0
		return
	}
	mb.mu.used -= size
}

type eventKey struct {
	goid           uint64
	stackByteDepth uint32
	probeID        uint32
}

type bufferedEvent struct {
	key   eventKey
	event dispatcher.Message
}

func cmpEventKey(a, b eventKey) int {
	return cmp.Or(
		cmp.Compare(a.goid, b.goid),
		cmp.Compare(a.stackByteDepth, b.stackByteDepth),
		cmp.Compare(a.probeID, b.probeID),
	)
}

func bufferedEventLess(a, b bufferedEvent) bool {
	return cmpEventKey(a.key, b.key) < 0
}

// bufferTree is a tree of buffered events.
type bufferTree struct {
	tree *btree.BTreeG[bufferedEvent]
	mb   *bufferedMessageTracker
}

func (bt *bufferTree) popMatchingEvent(key eventKey) (dispatcher.Message, bool) {
	got, ok := bt.tree.Delete(bufferedEvent{key: key, event: dispatcher.Message{}})
	if !ok {
		return dispatcher.Message{}, false
	}
	bt.mb.release(len(got.event.Event()))
	return got.event, true
}

var duplicateEventLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

func (bt *bufferTree) addEvent(key eventKey, event dispatcher.Message) (ok bool) {
	size := len(event.Event())
	if !bt.mb.add(size) {
		return false
	}
	if prev, ok := bt.tree.ReplaceOrInsert(bufferedEvent{
		key:   key,
		event: event,
	}); ok {
		if duplicateEventLogLimiter.Allow() {
			log.Warnf(
				"duplicate event for goid %d, stackByteDepth %d, probeID %d",
				key.goid, key.stackByteDepth, key.probeID,
			)
		} else {
			log.Tracef(
				"duplicate event for goid %d, stackByteDepth %d, probeID %d",
				key.goid, key.stackByteDepth, key.probeID,
			)
		}
		bt.mb.release(len(prev.event.Event()))
		return false
	}
	return true
}

func (bt *bufferTree) close() {
	var toRelease int
	bt.tree.Ascend(func(g bufferedEvent) bool {
		toRelease += len(g.event.Event())
		return true
	})
	bt.mb.release(toRelease)
	bt.tree.Clear(true)
}
