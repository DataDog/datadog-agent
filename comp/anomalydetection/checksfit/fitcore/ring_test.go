// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

var testDefault = ProtocolDescriptor{ID: array8("CORE0001"), Version: 2, MessageTypes: []uint32{1, 42}}
var testAlternate = ProtocolDescriptor{ID: array8("ALT00001"), Version: 2, MessageTypes: []uint32{42}}
var testOnlyOne = ProtocolDescriptor{ID: array8("ONE00001"), Version: 2, MessageTypes: []uint32{1}}

var testNextID atomic.Uint64

func testSessionID() uint64 {
	return uint64(time.Now().UnixNano()) + testNextID.Add(1)
}

// testPair creates a consumer-owned mapping and opens it again the way the
// producer does, without going through setup.
func testPair(t *testing.T, capacity int) (producer *mapping, consumer *mapping) {
	t.Helper()
	id := testSessionID()
	consumer, name, err := createMapping(id, capacity, 2)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	producer, err = openMapping(name, id, capacity, 2)
	if err != nil {
		t.Fatalf("open mapping: %v", err)
	}
	if err := consumer.unlinkName(); err != nil {
		t.Fatalf("unlink name: %v", err)
	}
	t.Cleanup(func() {
		producer.close()
		consumer.close()
	})
	return producer, consumer
}

func testRecord(payload []byte) Record {
	return Record{Kind: 1, Payload: payload}
}

func TestFullRingKeepsUnreadRecordsAndAcceptsOnlyPrefix(t *testing.T) {
	p, c := testPair(t, 64)
	a := bytes.Repeat([]byte{1}, 16)
	b := bytes.Repeat([]byte{2}, 16)
	d := bytes.Repeat([]byte{3}, 16)
	result, err := p.sendBatch([]Record{testRecord(a), testRecord(b), testRecord(d)}, testDefault, wakeWord)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 2 || result.Rejection != RejectionFull || result.NotificationError != nil {
		t.Fatalf("batch result = %+v", result)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, a) {
		t.Fatalf("first receive = %q, %v", payload, err)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, b) {
		t.Fatalf("second receive = %q, %v", payload, err)
	}
	result, err = p.sendBatch([]Record{testRecord(d)}, testDefault, wakeWord)
	if err != nil || result.Accepted != 1 {
		t.Fatalf("third send = %+v, %v", result, err)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, d) {
		t.Fatalf("third receive = %q, %v", payload, err)
	}
}

func TestPaddingOnlyPublicationAllowsALaterSend(t *testing.T) {
	p, c := testPair(t, 64)
	a := bytes.Repeat([]byte{1}, 16)
	b := bytes.Repeat([]byte{2}, 16)
	large := bytes.Repeat([]byte{9}, 24)
	result, err := p.sendBatch([]Record{testRecord(a), testRecord(b)}, testDefault, wakeWord)
	if err != nil || result.Accepted != 2 {
		t.Fatalf("first batch = %+v, %v", result, err)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, a) {
		t.Fatalf("first receive = %q, %v", payload, err)
	}
	result, err = p.sendBatch([]Record{testRecord(large)}, testDefault, wakeWord)
	if err != nil || result.Accepted != 0 || result.Rejection != RejectionFull {
		t.Fatalf("padding-only batch = %+v, %v", result, err)
	}
	if got := atomic.LoadUint32(p.word(writeOffset)); got != 0 {
		t.Fatalf("write index after wrap = %d", got)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, b) {
		t.Fatalf("second receive = %q, %v", payload, err)
	}
	result, err = p.sendBatch([]Record{testRecord(large)}, testDefault, wakeWord)
	if err != nil || result.Accepted != 1 {
		t.Fatalf("post-wrap batch = %+v, %v", result, err)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, large) {
		t.Fatalf("large receive = %q, %v", payload, err)
	}
}

func TestInvalidInputsAndCorruptHeadersFailWithoutReleasingBytes(t *testing.T) {
	p, c := testPair(t, 64)
	result, err := p.sendBatch([]Record{{Kind: 0, Payload: nil}}, testDefault, wakeWord)
	if err != nil || result.Rejection != RejectionInvalidType {
		t.Fatalf("zero kind = %+v, %v", result, err)
	}
	result, err = p.sendBatch([]Record{testRecord(bytes.Repeat([]byte{0}, 49))}, testDefault, wakeWord)
	if err != nil || result.Rejection != RejectionOversized {
		t.Fatalf("oversized = %+v, %v", result, err)
	}
	if got := atomic.LoadUint32(p.word(writeOffset)); got != 0 {
		t.Fatalf("write index moved to %d", got)
	}
	if _, err := p.sendBatch([]Record{testRecord(bytes.Repeat([]byte{4}, 8))}, testDefault, wakeWord); err != nil {
		t.Fatal(err)
	}
	// Corrupt the published record type; the consumer must fail without
	// moving the read index.
	binary.LittleEndian.PutUint32(c.region[ringOffset+4:ringOffset+8], 99)
	if _, _, _, err := c.receiveInner(testDefault, nil); err == nil {
		t.Fatal("corrupt header accepted")
	}
	if got := atomic.LoadUint32(c.word(readOffset)); got != 0 {
		t.Fatalf("read index moved to %d", got)
	}
}

func TestPublishedRecordRemainsAcceptedWhenWakeFails(t *testing.T) {
	p, c := testPair(t, 64)
	result, err := p.sendBatch([]Record{testRecord([]byte("published"))}, testDefault,
		func(*uint32) error { return errors.New("injected wake failure") })
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 1 || result.Rejection != RejectionNone {
		t.Fatalf("batch result = %+v", result)
	}
	if result.NotificationError == nil || result.NotificationError.Error() != "injected wake failure" {
		t.Fatalf("notification error = %v", result.NotificationError)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || string(payload) != "published" {
		t.Fatalf("receive = %q, %v", payload, err)
	}
}

func TestEmptyPayloadAndExactEndAreValid(t *testing.T) {
	p, c := testPair(t, 32)
	result, err := p.sendBatch([]Record{testRecord(nil), testRecord(bytes.Repeat([]byte{1}, 8))}, testDefault, wakeWord)
	if err != nil || result.Accepted != 2 {
		t.Fatalf("batch = %+v, %v", result, err)
	}
	if got := atomic.LoadUint32(p.word(writeOffset)); got != 24 {
		t.Fatalf("write index = %d, want 24", got)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || len(payload) != 0 {
		t.Fatalf("empty receive = %q, %v", payload, err)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, bytes.Repeat([]byte{1}, 8)) {
		t.Fatalf("second receive = %q, %v", payload, err)
	}
	result, err = p.sendBatch([]Record{testRecord(nil)}, testDefault, wakeWord)
	if err != nil || result.Accepted != 1 {
		t.Fatalf("wrap batch = %+v, %v", result, err)
	}
	if got := atomic.LoadUint32(p.word(writeOffset)); got != 0 {
		t.Fatalf("write index after wrap = %d", got)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || len(payload) != 0 {
		t.Fatalf("post-wrap receive = %q, %v", payload, err)
	}
}

func TestCapacityAndDerivedMaximumPayload(t *testing.T) {
	if err := validateCapacity(16); err != nil {
		t.Fatal(err)
	}
	if err := validateCapacity(1 << 30); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []int{0, 8, 17, (1 << 30) + 8} {
		if err := validateCapacity(bad); err == nil {
			t.Fatalf("capacity %d accepted", bad)
		}
	}
	p, c := testPair(t, 64)
	largest := bytes.Repeat([]byte{0xa5}, 48)
	result, err := p.sendBatch([]Record{testRecord(largest)}, testDefault, wakeWord)
	if err != nil || result.Accepted != 1 {
		t.Fatalf("largest = %+v, %v", result, err)
	}
	result, err = p.sendBatch([]Record{testRecord(bytes.Repeat([]byte{0}, 49))}, testDefault, wakeWord)
	if err != nil || result.Rejection != RejectionOversized {
		t.Fatalf("over-limit = %+v, %v", result, err)
	}
	if _, payload, _, err := c.receiveInner(testDefault, nil); err != nil || !bytes.Equal(payload, largest) {
		t.Fatalf("largest receive = %d bytes, %v", len(payload), err)
	}
}

func TestMixedRecordsSurviveRepeatedWrapsAndOwnedDataSurvivesReuse(t *testing.T) {
	p, c := testPair(t, 128)
	retained := 0
	for n := 0; n < 2000; n++ {
		a := bytes.Repeat([]byte{byte(n)}, (n*7)%49)
		b := bytes.Repeat([]byte{byte(n >> 8)}, (n*11)%41)
		result, err := p.sendBatch([]Record{testRecord(a), testRecord(b)}, testDefault, wakeWord)
		if err != nil {
			t.Fatal(err)
		}
		if result.NotificationError != nil {
			t.Fatal(result.NotificationError)
		}
		if result.Accepted >= 1 {
			_, payload, _, err := c.receiveInner(testDefault, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(payload, a) {
				t.Fatalf("iteration %d: payload mismatch", n)
			}
			retained++
		}
		if result.Accepted >= 2 {
			_, payload, _, err := c.receiveInner(testDefault, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(payload, b) {
				t.Fatalf("iteration %d: second payload mismatch", n)
			}
		}
	}
	if retained < 1000 {
		t.Fatalf("only %d first records accepted", retained)
	}
}

func TestWaitReturnsWhenSnapshotIsAlreadyStale(t *testing.T) {
	p, _ := testPair(t, 64)
	atomic.StoreUint32(p.word(writeOffset), 8)
	if err := waitWord(p.word(writeOffset), 0); err != nil {
		t.Fatalf("stale wait failed: %v", err)
	}
}

func TestSecondDescriptorSupportsType42WithoutCoreEdits(t *testing.T) {
	producer, consumer := testPair(t, 128)
	result, err := producer.sendBatch([]Record{{Kind: 42, Payload: []byte("forty-two")}}, testAlternate, wakeWord)
	if err != nil || result.Accepted != 1 {
		t.Fatalf("first 42 = %+v, %v", result, err)
	}
	kind, payload, _, err := consumer.receiveInner(testAlternate, nil)
	if err != nil || kind != 42 || string(payload) != "forty-two" {
		t.Fatalf("receive 42 = %d %q %v", kind, payload, err)
	}
	result, err = producer.sendBatch([]Record{{Kind: 42, Payload: []byte("again")}}, testAlternate, wakeWord)
	if err != nil || result.Accepted != 1 {
		t.Fatalf("second 42 = %+v, %v", result, err)
	}
	before := atomic.LoadUint32(consumer.word(readOffset))
	if _, _, _, err := consumer.receiveInner(testOnlyOne, nil); err == nil {
		t.Fatal("unknown type accepted by mismatched descriptor")
	}
	if got := atomic.LoadUint32(consumer.word(readOffset)); got != before {
		t.Fatalf("read index moved from %d to %d", before, got)
	}
	kind, payload, _, err = consumer.receiveInner(testAlternate, nil)
	if err != nil || kind != 42 || string(payload) != "again" {
		t.Fatalf("final receive = %d %q %v", kind, payload, err)
	}
}

func TestCancelledReceiveKeepsPublishedRecordAvailable(t *testing.T) {
	p, c := testPair(t, 64)
	if _, err := p.sendBatch([]Record{testRecord([]byte("retained"))}, testDefault, wakeWord); err != nil {
		t.Fatal(err)
	}
	reg := newWaitRegistry()
	if err := reg.cancel(); err != nil {
		t.Fatal(err)
	}
	_, _, cancelled, err := c.receiveInner(testDefault, reg)
	if err != nil || !cancelled {
		t.Fatalf("cancelled receive = %v (cancelled %v)", err, cancelled)
	}
	_, payload, _, err := c.receiveInner(testDefault, nil)
	if err != nil || string(payload) != "retained" {
		t.Fatalf("retained record = %q, %v", payload, err)
	}
}

func TestCancellationStopsABlockedReceiveWithoutReclaimingSpace(t *testing.T) {
	id := testSessionID()
	consumer, _, err := createMapping(id, 64, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { consumer.close() })
	readBefore := atomic.LoadUint32(consumer.word(readOffset))
	reg := newWaitRegistry()
	registered := make(chan struct{})
	done := make(chan bool, 1)
	go func() {
		close(registered)
		_, _, cancelled, err := consumer.receiveInner(testDefault, reg)
		if err != nil {
			t.Errorf("cancelled receive failed: %v", err)
			done <- false
			return
		}
		done <- cancelled
	}()
	<-registered
	// Give the receiver a moment to enter its native wait, then cancel.
	time.Sleep(20 * time.Millisecond)
	if err := reg.cancel(); err != nil {
		t.Fatal(err)
	}
	if !<-done {
		t.Fatal("receive did not report cancellation")
	}
	if readAfter := atomic.LoadUint32(consumer.word(readOffset)); readAfter != readBefore {
		t.Fatalf("read index moved from %d to %d", readBefore, readAfter)
	}
}

func TestCancelBeforeWaitPreventsRegistration(t *testing.T) {
	p, _ := testPair(t, 64)
	reg := newWaitRegistry()
	if err := reg.cancel(); err != nil {
		t.Fatal(err)
	}
	proceed, err := reg.waitOn(p.word(writeOffset), 0)
	if err != nil || proceed {
		t.Fatalf("waitOn after cancel = %v, %v", proceed, err)
	}
	if err := reg.cancel(); err != nil {
		t.Fatal(err)
	}
}

func TestSecondRegistrationOnOneRegistryFails(t *testing.T) {
	p, _ := testPair(t, 64)
	reg := newWaitRegistry()
	registered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := reg.waitOnWith(p.word(writeOffset), 0, func(word *uint32, expected uint32) error {
			// The first receiver is registered and held before its native
			// wait, so the second registration attempt is deterministically
			// concurrent.
			close(registered)
			<-release
			return waitWord(word, expected)
		})
		done <- err
	}()
	<-registered
	if _, err := reg.waitOn(p.word(writeOffset), 0); err == nil {
		t.Fatal("concurrent registration accepted")
	} else if !errors.Is(err, errSecondWaiter) {
		t.Fatalf("unexpected error: %v", err)
	}
	close(release)
	if err := reg.cancel(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestCancellationBetweenRegistrationAndNativeWaitIsNotLost ports the Rust
// token test: cancel's first wake fires while the receiver is held between
// registration and its native wait, and the retry loop must still deliver
// the cancellation once the receiver enters the wait.
func TestCancellationBetweenRegistrationAndNativeWaitIsNotLost(t *testing.T) {
	p, _ := testPair(t, 64)
	reg := newWaitRegistry()
	registered := make(chan struct{})
	release := make(chan struct{})
	workerDone := make(chan bool, 1)
	go func() {
		proceed, err := reg.waitOnWith(p.word(writeOffset), 0, func(word *uint32, expected uint32) error {
			close(registered)
			<-release
			return waitWord(word, expected)
		})
		if err != nil {
			t.Errorf("held wait failed: %v", err)
			workerDone <- false
			return
		}
		workerDone <- !proceed
	}()
	<-registered
	cancelDone := make(chan error, 1)
	go func() { cancelDone <- reg.cancel() }()
	// Let the first wake fire while the worker is still held before the
	// native wait, then let it proceed into the wait.
	time.Sleep(20 * time.Millisecond)
	close(release)
	select {
	case err := <-cancelDone:
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not return after the receiver entered its wait")
	}
	if !<-workerDone {
		t.Fatal("wake before the native wait was lost")
	}
}

func TestConcurrentSmallRingPreservesOrderAndPayloadAcrossWraps(t *testing.T) {
	capacity := 256
	id := testSessionID()
	consumer, name, err := createMapping(id, capacity, 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { consumer.close() })
	producerReady := make(chan struct{})
	producerDone := make(chan error, 1)
	go func() {
		producer, err := openMapping(name, id, capacity, 2)
		if err != nil {
			producerDone <- err
			return
		}
		defer producer.close()
		close(producerReady)
		for sequence := uint32(0); sequence < 10000; sequence++ {
			payload := make([]byte, 4+(int(sequence)*17)%96)
			binary.LittleEndian.PutUint32(payload, sequence)
			for index := 4; index < len(payload); index++ {
				payload[index] = byte(sequence % 251)
			}
			for {
				result, err := producer.sendBatch([]Record{testRecord(payload)}, testDefault, wakeWord)
				if err != nil {
					producerDone <- err
					return
				}
				if result.NotificationError != nil {
					producerDone <- result.NotificationError
					return
				}
				if result.Accepted == 1 {
					break
				}
				if result.Rejection != RejectionFull {
					producerDone <- fmt.Errorf("unexpected rejection %v", result.Rejection)
					return
				}
				runtime.Gosched()
			}
		}
		producerDone <- nil
	}()
	<-producerReady
	if err := consumer.unlinkName(); err != nil {
		t.Fatal(err)
	}
	for sequence := uint32(0); sequence < 10000; sequence++ {
		kind, payload, _, err := consumer.receiveInner(testDefault, nil)
		if err != nil {
			t.Fatal(err)
		}
		if kind != 1 {
			t.Fatalf("kind = %d", kind)
		}
		wantLen := 4 + (int(sequence)*17)%96
		if len(payload) != wantLen {
			t.Fatalf("sequence %d: payload length %d, want %d", sequence, len(payload), wantLen)
		}
		if got := binary.LittleEndian.Uint32(payload[:4]); got != sequence {
			t.Fatalf("sequence %d: header %d", sequence, got)
		}
		for _, value := range payload[4:] {
			if value != byte(sequence%251) {
				t.Fatalf("sequence %d: payload byte %d", sequence, value)
			}
		}
	}
	if err := <-producerDone; err != nil {
		t.Fatal(err)
	}
}

// The 4 GiB traffic test from the Rust suite is intentionally not ported;
// the physical wrap arithmetic is identical (indexes stay modulo capacity).
