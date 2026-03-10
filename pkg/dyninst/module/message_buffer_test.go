// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
)

func trackerUsed(mb *bufferedMessageTracker) int {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	return mb.mu.used
}

func makeMsg(size int) dispatcher.Message {
	return dispatcher.MakeTestingMessage(make([]byte, size))
}

func TestBufferTreeAddAndPop(t *testing.T) {
	mb := newBufferedMessageTracker(1024)
	bt := mb.newTree()
	defer bt.close()

	key := eventKey{goid: 1, stackByteDepth: 100, probeID: 5}

	ok := bt.addEvent(key, makeMsg(64))
	require.True(t, ok)
	assert.Equal(t, 64, trackerUsed(mb))

	msg, ok := bt.popMatchingEvent(key)
	require.True(t, ok)
	assert.Equal(t, 64, len(msg.Event()))
	assert.Equal(t, 0, trackerUsed(mb))
}

func TestBufferTreePopMissing(t *testing.T) {
	mb := newBufferedMessageTracker(1024)
	bt := mb.newTree()
	defer bt.close()

	key := eventKey{goid: 1, stackByteDepth: 100, probeID: 5}
	_, ok := bt.popMatchingEvent(key)
	assert.False(t, ok)
	assert.Equal(t, 0, trackerUsed(mb))
}

func TestBufferTreeBufferFull(t *testing.T) {
	mb := newBufferedMessageTracker(100)
	bt := mb.newTree()
	defer bt.close()

	key1 := eventKey{goid: 1, stackByteDepth: 100, probeID: 5}
	ok := bt.addEvent(key1, makeMsg(80))
	require.True(t, ok)
	assert.Equal(t, 80, trackerUsed(mb))

	// This should fail: 80 + 80 > 100.
	key2 := eventKey{goid: 2, stackByteDepth: 100, probeID: 5}
	ok = bt.addEvent(key2, makeMsg(80))
	assert.False(t, ok)
	assert.Equal(t, 80, trackerUsed(mb))
}

// TestBufferTreeDuplicateAccounting verifies that inserting a duplicate
// event returns true, properly accounts for the size change, and that
// popping the entry afterwards leaves used at zero. This is a
// regression test for an invariant violation where the duplicate path
// returned false, causing the caller to release a record still held
// by the tree.
func TestBufferTreeDuplicateAccounting(t *testing.T) {
	mb := newBufferedMessageTracker(1024)
	bt := mb.newTree()
	defer bt.close()

	key := eventKey{goid: 1, stackByteDepth: 100, probeID: 5}

	ok := bt.addEvent(key, makeMsg(64))
	require.True(t, ok)
	assert.Equal(t, 64, trackerUsed(mb))

	// Insert a duplicate with a different size.
	ok = bt.addEvent(key, makeMsg(48))
	require.True(t, ok, "duplicate should return true")
	assert.Equal(t, 48, trackerUsed(mb),
		"used should reflect the replacement event size")

	// Pop should return the replacement and bring used back to zero.
	msg, ok := bt.popMatchingEvent(key)
	require.True(t, ok)
	assert.Equal(t, 48, len(msg.Event()))
	assert.Equal(t, 0, trackerUsed(mb),
		"used must be zero after popping the only event")
}

func TestBufferTreeDuplicateSameSize(t *testing.T) {
	mb := newBufferedMessageTracker(1024)
	bt := mb.newTree()
	defer bt.close()

	key := eventKey{goid: 1, stackByteDepth: 100, probeID: 5}

	ok := bt.addEvent(key, makeMsg(64))
	require.True(t, ok)

	ok = bt.addEvent(key, makeMsg(64))
	require.True(t, ok)
	assert.Equal(t, 64, trackerUsed(mb))

	msg, ok := bt.popMatchingEvent(key)
	require.True(t, ok)
	assert.Equal(t, 64, len(msg.Event()))
	assert.Equal(t, 0, trackerUsed(mb))
}

func TestBufferTreeCloseReleasesAccounting(t *testing.T) {
	mb := newBufferedMessageTracker(1024)
	bt := mb.newTree()

	keys := []eventKey{
		{goid: 1, stackByteDepth: 10, probeID: 1},
		{goid: 2, stackByteDepth: 20, probeID: 2},
		{goid: 3, stackByteDepth: 30, probeID: 3},
	}
	for _, k := range keys {
		require.True(t, bt.addEvent(k, makeMsg(32)))
	}
	assert.Equal(t, 96, trackerUsed(mb))

	bt.close()
	assert.Equal(t, 0, trackerUsed(mb),
		"close must release all tracked bytes")
}

func TestBufferTreeMultipleTreesShareTracker(t *testing.T) {
	mb := newBufferedMessageTracker(128)
	bt1 := mb.newTree()
	bt2 := mb.newTree()
	defer bt1.close()
	defer bt2.close()

	key1 := eventKey{goid: 1, stackByteDepth: 10, probeID: 1}
	key2 := eventKey{goid: 2, stackByteDepth: 20, probeID: 2}

	require.True(t, bt1.addEvent(key1, makeMsg(64)))
	require.True(t, bt2.addEvent(key2, makeMsg(64)))
	assert.Equal(t, 128, trackerUsed(mb))

	// Tracker is full; a third add on either tree should fail.
	key3 := eventKey{goid: 3, stackByteDepth: 30, probeID: 3}
	assert.False(t, bt1.addEvent(key3, makeMsg(1)))

	// Releasing from one tree frees space for the other.
	_, ok := bt1.popMatchingEvent(key1)
	require.True(t, ok)
	assert.Equal(t, 64, trackerUsed(mb))
	require.True(t, bt1.addEvent(key3, makeMsg(32)))
	assert.Equal(t, 96, trackerUsed(mb))
}
