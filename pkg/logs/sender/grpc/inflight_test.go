// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Helper function to create test payloads
func createTestPayload(content string) *message.Payload {
	return &message.Payload{
		Encoded: []byte(content),
	}
}

func TestNewInflightTracker(t *testing.T) {
	tracker := newInflightTracker("test", 10)

	assert.NotNil(t, tracker)
	assert.Equal(t, 10, tracker.cap)
	assert.Equal(t, 0, tracker.head)
	assert.Equal(t, 0, tracker.sentTail)
	assert.Equal(t, 0, tracker.tail)
	assert.Equal(t, uint32(0), tracker.headBatchID)
	assert.Equal(t, uint32(0), tracker.batchIDCounter)
	assert.True(t, tracker.hasSpace())
	assert.False(t, tracker.hasUnacked())
	assert.False(t, tracker.hasUnSent())
}

func TestInflightTrackerAppend(t *testing.T) {
	tracker := newInflightTracker("test", 10)

	// Append first payload
	payload1 := createTestPayload("test1")
	assert.True(t, tracker.append(payload1))
	assert.Equal(t, 1, tracker.totalCount())
	assert.True(t, tracker.hasUnSent())
	assert.False(t, tracker.hasUnacked())

	// Append second payload
	payload2 := createTestPayload("test2")
	assert.True(t, tracker.append(payload2))
	assert.Equal(t, 2, tracker.totalCount())
	assert.True(t, tracker.hasSpace())

	// Append third payload
	payload3 := createTestPayload("test3")
	assert.True(t, tracker.append(payload3))
	assert.Equal(t, 3, tracker.totalCount())
}

func TestInflightTrackerAppendWhenFull(t *testing.T) {
	// Test filling buffer to absolute capacity from empty state
	tracker := newInflightTracker("test", 3)

	// Fill to capacity (3 items)
	assert.True(t, tracker.append(createTestPayload("test1")))
	assert.Equal(t, 1, tracker.totalCount())
	assert.True(t, tracker.hasSpace())

	assert.True(t, tracker.append(createTestPayload("test2")))
	assert.Equal(t, 2, tracker.totalCount())
	assert.True(t, tracker.hasSpace())

	assert.True(t, tracker.append(createTestPayload("test3")))
	assert.Equal(t, 3, tracker.totalCount())
	assert.False(t, tracker.hasSpace())

	// Append should fail when full
	assert.False(t, tracker.append(createTestPayload("test4")))
	assert.Equal(t, 3, tracker.totalCount())
}

func TestInflightTrackerMarkSent(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Add buffered payloads
	payload1 := createTestPayload("test1")
	payload2 := createTestPayload("test2")
	tracker.append(payload1)
	tracker.append(payload2)

	assert.Equal(t, 0, tracker.sentCount())
	assert.True(t, tracker.hasUnSent())
	assert.False(t, tracker.hasUnacked())

	// Mark first as sent
	assert.True(t, tracker.markSent())
	assert.Equal(t, 1, tracker.sentCount())
	assert.Equal(t, uint32(0), tracker.getHeadBatchID())
	assert.Equal(t, uint32(1), tracker.nextBatchID())
	assert.True(t, tracker.hasUnacked())
	assert.True(t, tracker.hasUnSent())

	// Mark second as sent
	assert.True(t, tracker.markSent())
	assert.Equal(t, 2, tracker.sentCount())
	assert.Equal(t, uint32(0), tracker.getHeadBatchID())
	assert.Equal(t, uint32(2), tracker.nextBatchID())
	assert.True(t, tracker.hasUnacked())
	assert.False(t, tracker.hasUnSent())

	// Try to mark sent when no buffered items
	assert.False(t, tracker.markSent())
}

func TestInflightTrackerPop(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Add and mark payloads as sent
	payload1 := createTestPayload("test1")
	payload2 := createTestPayload("test2")
	tracker.append(payload1)
	tracker.append(payload2)
	tracker.markSent()
	tracker.markSent()

	assert.Equal(t, 2, tracker.sentCount())
	assert.Equal(t, uint32(0), tracker.getHeadBatchID())

	// Pop first payload
	popped1 := tracker.pop()
	assert.Equal(t, payload1, popped1)
	assert.Equal(t, 1, tracker.sentCount())
	assert.Equal(t, uint32(1), tracker.getHeadBatchID())
	assert.True(t, tracker.hasUnacked())

	// Pop second payload
	popped2 := tracker.pop()
	assert.Equal(t, payload2, popped2)
	assert.Equal(t, 0, tracker.sentCount())
	assert.False(t, tracker.hasUnacked())

	// Pop when empty should return nil
	poppedNil := tracker.pop()
	assert.Nil(t, poppedNil)
}

func TestInflightTrackerNextToSend(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// NextToSend on empty tracker should return nil
	assert.Nil(t, tracker.nextToSend())

	// Add buffered payloads
	payload1 := createTestPayload("test1")
	payload2 := createTestPayload("test2")
	tracker.append(payload1)
	tracker.append(payload2)

	// NextToSend should return first buffered payload
	next := tracker.nextToSend()
	assert.Equal(t, payload1, next)

	// Mark first as sent
	tracker.markSent()

	// NextToSend should return second buffered payload
	next = tracker.nextToSend()
	assert.Equal(t, payload2, next)

	// Mark second as sent
	tracker.markSent()

	// NextToSend should return nil when no buffered payloads
	next = tracker.nextToSend()
	assert.Nil(t, next)
}

func TestInflightTrackerBatchIDSequence(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Add and send payloads
	for i := 0; i < 3; i++ {
		payload := createTestPayload("test")
		tracker.append(payload)
	}

	// Initial batchIDCounter should be 0
	assert.Equal(t, uint32(0), tracker.nextBatchID())

	// Mark first as sent
	tracker.markSent()
	assert.Equal(t, uint32(0), tracker.getHeadBatchID())
	assert.Equal(t, uint32(1), tracker.nextBatchID())

	// Mark second as sent
	tracker.markSent()
	assert.Equal(t, uint32(0), tracker.getHeadBatchID())
	assert.Equal(t, uint32(2), tracker.nextBatchID())

	// Mark third as sent
	tracker.markSent()
	assert.Equal(t, uint32(0), tracker.getHeadBatchID())
	assert.Equal(t, uint32(3), tracker.nextBatchID())

	// Pop first - headBatchID should advance
	tracker.pop()
	assert.Equal(t, uint32(1), tracker.getHeadBatchID())

	// Pop second - headBatchID should advance
	tracker.pop()
	assert.Equal(t, uint32(2), tracker.getHeadBatchID())
}

func TestInflightTrackerResetOnRotation(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Add payloads and mark some as sent
	for i := 0; i < 3; i++ {
		payload := createTestPayload("test")
		tracker.append(payload)
		tracker.markSent()
	}

	// Pop one ack
	tracker.pop()

	// State before reset: 2 sent (awaiting ack), 0 buffered
	assert.Equal(t, 2, tracker.sentCount())
	assert.Equal(t, 0, tracker.totalCount()-tracker.sentCount())
	assert.Equal(t, uint32(1), tracker.getHeadBatchID())
	assert.Equal(t, uint32(3), tracker.nextBatchID())

	// Reset on rotation
	tracker.resetOnRotation()

	// After reset: 0 sent, 2 buffered (un-acked payloads become buffered)
	assert.Equal(t, 0, tracker.sentCount())
	assert.Equal(t, 2, tracker.totalCount())
	assert.True(t, tracker.hasUnSent())
	assert.False(t, tracker.hasUnacked())

	// Batch IDs should reset to 1
	assert.Equal(t, uint32(1), tracker.headBatchID)
	assert.Equal(t, uint32(1), tracker.nextBatchID())
}

func TestInflightTrackerWrapAround(t *testing.T) {
	// Test wrap-around behavior without filling to absolute capacity
	tracker := newInflightTracker("test", 6)

	// Fill and empty to advance head pointer
	payload1 := createTestPayload("test1")
	payload2 := createTestPayload("test2")

	// Add, send, and ack first two to advance pointers
	tracker.append(payload1)
	tracker.markSent()
	tracker.pop()

	tracker.append(payload2)
	tracker.markSent()
	tracker.pop()

	// Now add more items that will wrap around in the ring buffer
	payload3 := createTestPayload("test3")
	payload4 := createTestPayload("test4")
	payload5 := createTestPayload("test5")

	assert.True(t, tracker.append(payload3))
	assert.True(t, tracker.append(payload4))
	assert.True(t, tracker.append(payload5))

	assert.Equal(t, 3, tracker.totalCount())
	assert.True(t, tracker.hasSpace())

	// Mark all as sent and pop them
	tracker.markSent()
	tracker.markSent()
	tracker.markSent()

	popped3 := tracker.pop()
	popped4 := tracker.pop()
	popped5 := tracker.pop()

	assert.Equal(t, payload3, popped3)
	assert.Equal(t, payload4, popped4)
	assert.Equal(t, payload5, popped5)
	assert.Equal(t, 0, tracker.totalCount())
}

func TestInflightTrackerSentCount(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Initially no sent items
	assert.Equal(t, 0, tracker.sentCount())

	// Add buffered payloads
	tracker.append(createTestPayload("test1"))
	tracker.append(createTestPayload("test2"))
	tracker.append(createTestPayload("test3"))

	assert.Equal(t, 0, tracker.sentCount())

	// Mark as sent
	tracker.markSent()
	assert.Equal(t, 1, tracker.sentCount())

	tracker.markSent()
	assert.Equal(t, 2, tracker.sentCount())

	// Pop one
	tracker.pop()
	assert.Equal(t, 1, tracker.sentCount())

	// Mark another as sent
	tracker.markSent()
	assert.Equal(t, 2, tracker.sentCount())
}

func TestInflightTrackerTotalCount(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Initially empty
	assert.Equal(t, 0, tracker.totalCount())

	// Add buffered payloads
	tracker.append(createTestPayload("test1"))
	assert.Equal(t, 1, tracker.totalCount())

	tracker.append(createTestPayload("test2"))
	assert.Equal(t, 2, tracker.totalCount())

	// Mark both as sent (doesn't change total count)
	tracker.markSent()
	tracker.markSent()
	assert.Equal(t, 2, tracker.totalCount())

	// Pop reduces total count
	tracker.pop()
	assert.Equal(t, 1, tracker.totalCount())

	tracker.pop()
	assert.Equal(t, 0, tracker.totalCount())
}

func TestInflightTrackerHasSpace(t *testing.T) {
	tracker := newInflightTracker("test", 10)

	// Initially has space
	assert.True(t, tracker.hasSpace())

	// Add several items
	for i := 0; i < 5; i++ {
		tracker.append(createTestPayload("test"))
	}
	assert.True(t, tracker.hasSpace())

	// Pop one to verify space tracking
	tracker.markSent()
	tracker.pop()
	assert.True(t, tracker.hasSpace())
}

func TestInflightTrackerMixedOperations(t *testing.T) {
	// Test a realistic sequence of operations
	tracker := newInflightTracker("test", 5)

	// Add 3 buffered payloads
	p1 := createTestPayload("msg1")
	p2 := createTestPayload("msg2")
	p3 := createTestPayload("msg3")

	tracker.append(p1)
	tracker.append(p2)
	tracker.append(p3)

	assert.Equal(t, 3, tracker.totalCount())
	assert.Equal(t, 0, tracker.sentCount())

	// Send first 2
	tracker.markSent()
	tracker.markSent()

	assert.Equal(t, 3, tracker.totalCount())
	assert.Equal(t, 2, tracker.sentCount())
	assert.True(t, tracker.hasUnacked())
	assert.True(t, tracker.hasUnSent())

	// Receive ack for first
	popped := tracker.pop()
	assert.Equal(t, p1, popped)
	assert.Equal(t, 2, tracker.totalCount())
	assert.Equal(t, 1, tracker.sentCount())

	// Add more payloads
	p4 := createTestPayload("msg4")
	p5 := createTestPayload("msg5")
	tracker.append(p4)
	tracker.append(p5)

	assert.Equal(t, 4, tracker.totalCount())
	assert.Equal(t, 1, tracker.sentCount())

	// Send remaining buffered
	tracker.markSent() // p3
	tracker.markSent() // p4
	tracker.markSent() // p5

	assert.Equal(t, 4, tracker.totalCount())
	assert.Equal(t, 4, tracker.sentCount())
	assert.False(t, tracker.hasUnSent())

	// Receive all remaining acks
	assert.Equal(t, p2, tracker.pop())
	assert.Equal(t, p3, tracker.pop())
	assert.Equal(t, p4, tracker.pop())
	assert.Equal(t, p5, tracker.pop())

	assert.Equal(t, 0, tracker.totalCount())
	assert.False(t, tracker.hasUnacked())
}

func TestInflightTrackerResetOnRotationWithBuffered(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Mix of sent and buffered payloads
	tracker.append(createTestPayload("msg1"))
	tracker.append(createTestPayload("msg2"))
	tracker.append(createTestPayload("msg3"))
	tracker.append(createTestPayload("msg4"))

	// Send first two
	tracker.markSent()
	tracker.markSent()

	// Ack first one
	tracker.pop()

	// State: 1 sent, 2 buffered, total 3
	assert.Equal(t, 1, tracker.sentCount())
	assert.Equal(t, 3, tracker.totalCount())

	// Reset on rotation
	tracker.resetOnRotation()

	// All items should be buffered now
	assert.Equal(t, 0, tracker.sentCount())
	assert.Equal(t, 3, tracker.totalCount())
	assert.True(t, tracker.hasUnSent())
	assert.False(t, tracker.hasUnacked())

	// Batch IDs reset
	assert.Equal(t, uint32(1), tracker.nextBatchID())
}

func TestInflightTrackerBatchIDAfterRotation(t *testing.T) {
	tracker := newInflightTracker("test", 5)

	// Add and send some payloads
	tracker.append(createTestPayload("msg1"))
	tracker.append(createTestPayload("msg2"))
	tracker.markSent()
	tracker.markSent()

	assert.Equal(t, uint32(0), tracker.getHeadBatchID())
	assert.Equal(t, uint32(2), tracker.nextBatchID())

	// Reset on rotation
	tracker.resetOnRotation()

	// Batch IDs should reset to 1 (0 is reserved for snapshot)
	assert.Equal(t, uint32(1), tracker.nextBatchID())

	// Send items with new batch IDs
	tracker.markSent()
	assert.Equal(t, uint32(1), tracker.getHeadBatchID())
	assert.Equal(t, uint32(2), tracker.nextBatchID())

	tracker.markSent()
	assert.Equal(t, uint32(1), tracker.getHeadBatchID())
	assert.Equal(t, uint32(3), tracker.nextBatchID())
}
