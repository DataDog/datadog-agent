// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// inflightTracker is a bounded FIFO queue that tracks payloads in two regions:
// 1. Sent but awaiting acknowledgment (head to sentTail)
// 2. Buffered but not yet sent to the network (sentTail to tail)
//
// Queue Layout:
// [--sent awaiting ack--][--buffered not sent--]
// ^                      ^                      ^
// head                 sentTail                 tail
//
// BatchID tracking:
// - Sent payloads have sequential batchIDs: [headBatchID, headBatchID+1, ..., headBatchID+sentSize-1]
// - Only tracks headBatchID (oldest sent) and nextBatchID (next to be assigned)
//
// Snapshot State:
// - Maintains accumulated state changes for stream bootstrapping
// - Represents the state "before" the first payload in the queue
// - Updated when payloads are acknowledged (popped)
type inflightTracker struct {
	workerID       string
	items          []*message.Payload
	head           int           // Index of the oldest sent item (awaiting ack)
	sentTail       int           // Index of the first buffered item that's not yet sent
	tail           int           // Index of the next available slot for new buffered items
	cap            int           // Maximum total capacity of the tracker
	headBatchID    uint32        // BatchID of the oldest sent payload (at head)
	batchIDCounter uint32        // Next batchID to be assigned when markSent is called
	planner        *statePlanner // Owns snapshot state and per-stream state planning
}

// newInflightTracker creates a new bounded inflight tracker with the given capacity
// Allocates capacity+1 slots to implement the "waste one slot" ring buffer pattern
func newInflightTracker(workerID string, capacity int) *inflightTracker {
	return &inflightTracker{
		workerID: workerID,
		items:    make([]*message.Payload, capacity+1),
		cap:      capacity,
		planner:  newStatePlanner(),
	}
}

// hasSpace returns true if there is at least one free slot
func (t *inflightTracker) hasSpace() bool {
	return t.totalCount() < t.cap
}

// append adds a new payload to the buffered region (not yet sent)
// Returns true if the payload was added, false if the tracker is full
func (t *inflightTracker) append(payload *message.Payload) bool {
	if !t.hasSpace() {
		return false
	}
	t.items[t.tail] = payload
	tlmWorkerInflightSize.Add(float64(len(payload.Encoded)), t.workerID)
	t.tail = (t.tail + 1) % len(t.items)
	return true
}

// pop removes and returns the oldest sent payload (at head) after receiving an ack
// Returns nil if there are no sent payloads
// Also applies any state changes from the payload to the snapshot state
func (t *inflightTracker) pop() *message.Payload {
	if t.head == t.sentTail {
		return nil
	}
	payload := t.items[t.head]
	t.items[t.head] = nil // Allow GC
	t.head = (t.head + 1) % len(t.items)

	tlmWorkerInflightSize.Sub(float64(len(payload.Encoded)), t.workerID)

	// Apply state changes from this payload to snapshot
	if payload.StatefulExtra != nil {
		if extra, ok := payload.StatefulExtra.(*StatefulExtra); ok {
			if refs, ok := t.inflightReferences(); ok {
				t.planner.applyAcked(extra, &refs)
			} else {
				t.planner.applyAcked(extra, nil)
			}
		}
	}

	// Advance headBatchID for the next payload
	if t.head != t.sentTail {
		t.headBatchID++
	}

	return payload
}

// hasUnacked returns true if there are sent payloads awaiting acknowledgment
func (t *inflightTracker) hasUnacked() bool {
	return t.head != t.sentTail
}

// hasUnSent returns true if there are buffered payloads not yet sent
func (t *inflightTracker) hasUnSent() bool {
	return t.sentTail != t.tail
}

// getHeadBatchID returns the expected batchID at the head (oldest sent payload)
// Caller must check hasUnacked() first to ensure there are sent payloads
func (t *inflightTracker) getHeadBatchID() uint32 {
	return t.headBatchID
}

// nextBatchID returns the batchID that will be assigned to the next sent item
// This is a peek operation (idempotent, no mutation)
func (t *inflightTracker) nextBatchID() uint32 {
	return t.batchIDCounter
}

// markSent moves a buffered payload to the sent region and assigns it a batchID
// Returns true if successful, false if there are no buffered payloads
func (t *inflightTracker) markSent() bool {
	if t.sentTail == t.tail {
		return false
	}

	if extra, ok := t.items[t.sentTail].StatefulExtra.(*StatefulExtra); ok && extra != nil {
		t.planner.markSent(extra.WireDatums)
	}

	// If this is the first sent item, set headBatchID
	if t.head == t.sentTail {
		t.headBatchID = t.batchIDCounter
	}

	t.sentTail = (t.sentTail + 1) % len(t.items)
	t.batchIDCounter++ // Increment counter for next batch
	return true
}

// nextToSend returns the next buffered payload ready to be sent (without removing it)
// Returns nil if there are no buffered payloads
func (t *inflightTracker) nextToSend() *message.Payload {
	if t.sentTail == t.tail {
		return nil
	}
	return t.items[t.sentTail]
}

func (t *inflightTracker) nextToSendEncoded(compressor compression.Compressor) ([]byte, error) {
	payload := t.nextToSend()
	if payload == nil {
		return nil, nil
	}

	extra, ok := payload.StatefulExtra.(*StatefulExtra)
	if !ok || extra == nil || len(extra.WireDatums) == 0 {
		return payload.Encoded, nil
	}

	datums, changed := t.planner.planWireDatums(extra.WireDatums)
	if !changed {
		return payload.Encoded, nil
	}
	datums = deltaEncodeDatumsForWire(datums)

	serialized, err := proto.Marshal(&statefulpb.DatumSequence{Data: datums})
	if err != nil {
		return nil, err
	}
	return compressor.Compress(serialized)
}

// sentCount returns the number of sent payloads awaiting ack
func (t *inflightTracker) sentCount() int {
	return (t.sentTail - t.head + len(t.items)) % len(t.items)
}

// totalCount returns the total number of tracked payloads
func (t *inflightTracker) totalCount() int {
	return (t.tail - t.head + len(t.items)) % len(t.items)
}

// resetOnRotation set any un-acked payload as un-sent and reset the batchID.
func (t *inflightTracker) resetOnRotation() {
	// Move all sent items back to buffered region by resetting sentTail to head
	// This makes all items [head, tail) buffered again
	t.sentTail = t.head

	// Reset batchID counter for the new stream
	// Make the first batchID be 1, 0 is reserved for the snapshot state
	t.headBatchID = 1
	t.batchIDCounter = 1
	t.planner.resetStream()
}

// getSnapshot returns the current snapshot state for stream bootstrapping
// Returns serialized bytes (marshaled DatumSequence) or nil if empty
func (t *inflightTracker) getSnapshot() []byte {
	refs, ok := t.inflightReferences()
	if !ok {
		serialized, _ := t.planner.snapshotBytes(nil)
		return serialized
	}

	serialized, _ := t.planner.snapshotBytes(&refs)
	return serialized
}

func (t *inflightTracker) resetStreamSent() {
	t.planner.resetStream()
}

func (t *inflightTracker) inflightReferences() (stateReferences, bool) {
	refs := newStateReferences()
	for count, index := 0, t.head; count < t.totalCount(); count++ {
		payload := t.items[index]
		extra, ok := payload.StatefulExtra.(*StatefulExtra)
		if !ok || extra == nil || extra.WireDatums == nil {
			return stateReferences{}, false
		}
		addDatumReferences(refs, extra.WireDatums)
		index = (index + 1) % len(t.items)
	}
	return refs, true
}
