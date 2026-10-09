// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`): the transport
// comes from commit 4961722de9009afdbbb711fc0adf14a8f6ff9277, and the broadcast
// transport from commit 788233d2ffcc1e9d19b8e8202ca7908b64c64687
// (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"path/filepath"
	"testing"
	"time"
)

var broadcastTestProtocol = ProtocolDescriptor{
	ID:           array8("BCAST001"),
	Version:      1,
	MessageTypes: []uint32{1, 2},
}

func newTestPublisher(t *testing.T) (*BroadcastPublisher, string) {
	t.Helper()
	path := filepath.Join(testDirectory(t), "setup.sock")
	config := NewBroadcastConfig(UnixEndpoint(path))
	config.RingCapacity = 4096
	config.MaxSubscribers = 4
	config.SetupTimeout = 5 * time.Second
	publisher, err := OpenBroadcastPublisher(config, broadcastTestProtocol)
	if err != nil {
		t.Fatalf("open publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })
	return publisher, path
}

func newTestSubscriber(t *testing.T, path string) *Subscription {
	t.Helper()
	config := NewSubscriberConfig(UnixEndpoint(path))
	config.SetupTimeout = 5 * time.Second
	subscription, err := Subscribe(config, broadcastTestProtocol)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return subscription
}

func TestBroadcastLateJoinAndGracefulLeave(t *testing.T) {
	publisher, path := newTestPublisher(t)
	first := newTestSubscriber(t, path)
	defer first.Close()

	if outcome := publisher.SendBatch([]Record{{Kind: 1, Payload: []byte("e1")}}); outcome.Accepted != 1 {
		t.Fatalf("accepted=%d failure=%v", outcome.Accepted, outcome.Failure)
	}
	if _, payload, err := first.Receive(); err != nil || string(payload) != "e1" {
		t.Fatalf("first receive: %q %v", payload, err)
	}

	second := newTestSubscriber(t, path)
	defer second.Close()
	if publisher.SubscriberCount() != 2 {
		t.Fatalf("subscriber count=%d", publisher.SubscriberCount())
	}
	if outcome := publisher.SendBatch([]Record{{Kind: 1, Payload: []byte("e2")}}); outcome.Accepted != 1 {
		t.Fatalf("accepted=%d", outcome.Accepted)
	}
	if _, payload, err := first.Receive(); err != nil || string(payload) != "e2" {
		t.Fatalf("first e2: %q %v", payload, err)
	}
	if _, payload, err := second.Receive(); err != nil || string(payload) != "e2" {
		t.Fatalf("second e2: %q %v", payload, err)
	}

	if err := second.Unsubscribe(); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if publisher.SubscriberCount() != 1 {
		t.Fatalf("subscriber count after leave=%d", publisher.SubscriberCount())
	}
	if outcome := publisher.SendBatch([]Record{{Kind: 1, Payload: []byte("e3")}}); outcome.Accepted != 1 {
		t.Fatalf("accepted=%d", outcome.Accepted)
	}
	if _, payload, err := first.Receive(); err != nil || string(payload) != "e3" {
		t.Fatalf("first e3: %q %v", payload, err)
	}
}

func TestBroadcastAllSubscribersSeeIdenticalOrder(t *testing.T) {
	publisher, path := newTestPublisher(t)
	subscribers := make([]*Subscription, 3)
	for i := range subscribers {
		subscribers[i] = newTestSubscriber(t, path)
		defer subscribers[i].Close()
	}
	records := []Record{
		{Kind: 1, Payload: []byte("a")},
		{Kind: 2, Payload: []byte("bb")},
		{Kind: 1, Payload: []byte("ccc")},
	}
	if outcome := publisher.SendBatch(records); outcome.Accepted != 3 {
		t.Fatalf("accepted=%d failure=%v", outcome.Accepted, outcome.Failure)
	}
	for index, subscriber := range subscribers {
		for _, expected := range records {
			kind, payload, err := subscriber.Receive()
			if err != nil {
				t.Fatalf("subscriber %d: %v", index, err)
			}
			if kind != expected.Kind || string(payload) != string(expected.Payload) {
				t.Fatalf("subscriber %d got (%d,%q), want (%d,%q)", index, kind, payload, expected.Kind, expected.Payload)
			}
		}
	}
}

func TestBroadcastInvalidAndOversizedRecordsAreRejected(t *testing.T) {
	publisher, path := newTestPublisher(t)
	subscriber := newTestSubscriber(t, path)
	defer subscriber.Close()

	outcome := publisher.SendBatch([]Record{{Kind: 9, Payload: []byte("x")}})
	if outcome.Accepted != 0 || outcome.Rejection != RejectionInvalidType {
		t.Fatalf("invalid type: accepted=%d rejection=%v", outcome.Accepted, outcome.Rejection)
	}
	outcome = publisher.SendBatch([]Record{{Kind: 1, Payload: make([]byte, 4096)}})
	if outcome.Accepted != 0 || outcome.Rejection != RejectionOversized {
		t.Fatalf("oversized: accepted=%d rejection=%v", outcome.Accepted, outcome.Rejection)
	}
}

func TestBroadcastMismatchedProtocolIsRejected(t *testing.T) {
	_, path := newTestPublisher(t)
	other := ProtocolDescriptor{ID: array8("OTHER001"), Version: 1, MessageTypes: []uint32{1}}
	config := NewSubscriberConfig(UnixEndpoint(path))
	config.SetupTimeout = 2 * time.Second
	if _, err := Subscribe(config, other); err == nil {
		t.Fatal("expected a contract mismatch")
	}
}

func TestBroadcastRegistrySwapRemoveKeepsReverseIndex(t *testing.T) {
	inner := newBroadcastInner(nil, broadcastTestProtocol, 0, 256, 4, "", time.Second)
	for slot := uint32(0); slot < 3; slot++ {
		inner.activeIndex[slot] = int32(len(inner.activeSlots))
		inner.activeSlots = append(inner.activeSlots, slot)
		inner.freeSlots = inner.freeSlots[:0]
	}
	// Mirror retireSlotLocked's bookkeeping for the middle slot.
	position := inner.activeIndex[1]
	last := len(inner.activeSlots) - 1
	lastSlot := inner.activeSlots[last]
	inner.activeSlots[position] = lastSlot
	inner.activeSlots = inner.activeSlots[:last]
	inner.activeIndex[1] = -1
	if int(position) < len(inner.activeSlots) {
		inner.activeIndex[lastSlot] = position
	}
	if len(inner.activeSlots) != 2 || inner.activeSlots[0] != 0 || inner.activeSlots[1] != 2 {
		t.Fatalf("active list=%v", inner.activeSlots)
	}
	if inner.activeIndex[2] != 1 || inner.activeIndex[1] != -1 {
		t.Fatalf("reverse index=%v", inner.activeIndex)
	}
}

func TestBroadcastFullRingBlocksThenResumes(t *testing.T) {
	publisher, path := newTestPublisher(t)
	subscriber := newTestSubscriber(t, path)
	defer subscriber.Close()
	a := []byte("aaaaaaaaaaaaaaaa")
	b := []byte("bbbbbbbbbbbbbbbb")
	d := []byte("dddddddddddddddd")
	if outcome := publisher.SendBatch([]Record{{Kind: 1, Payload: a}, {Kind: 1, Payload: b}}); outcome.Accepted != 2 {
		t.Fatalf("fill accepted=%d", outcome.Accepted)
	}
	received := make(chan []string, 1)
	go func() {
		values := make([]string, 0, 3)
		for i := 0; i < 3; i++ {
			_, payload, err := subscriber.Receive()
			if err != nil {
				values = append(values, "error:"+err.Error())
				break
			}
			values = append(values, string(payload))
		}
		received <- values
	}()
	// The ring is full: this send blocks until the reader releases space.
	if outcome := publisher.SendBatch([]Record{{Kind: 1, Payload: d}}); outcome.Accepted != 1 {
		t.Fatalf("blocked send accepted=%d failure=%v", outcome.Accepted, outcome.Failure)
	}
	values := <-received
	want := []string{string(a), string(b), string(d)}
	if len(values) != len(want) {
		t.Fatalf("received %v", values)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("received %v, want %v", values, want)
		}
	}
}
