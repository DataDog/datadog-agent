// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checksfit

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
)

// goldenEventBytes is the cross-language pin: the Rust implementation in the
// checks-protocol crate asserts this same literal, so a divergence in field
// order, prefixes, or endianness fails on one side or the other.
func goldenEventBytes() []byte {
	return []byte{
		26, 0, 0, 0, // title length
		'A', 'A', 'D', ' ', 'a', 'n', 'o', 'm', 'a', 'l', 'y', ':', ' ', 's', 'y', 's', 't', 'e', 'm', '.',
		'l', 'o', 'a', 'd', '.', '1', // "AAD anomaly: system.load.1"
		39, 0, 0, 0, // description length
		's', 'e', 'v', 'e', 'r', 'i', 't', 'y', '=', 'm', 'e', 'd', 'i', 'u', 'm', ' ', 's', 'c', 'o', 'r', 'e',
		'=', '0', '.', '8', '7', ' ', 'h', 'o', 's', 't', '=', 'm', 'y', '-', 'h', 'o', 's', 't',
		// "severity=medium score=0.87 host=my-host"
		8, 7, 6, 5, 4, 3, 2, 1, // timestamp
	}
}

func goldenEvent() AnomalyEvent {
	return AnomalyEvent{
		Title:       "AAD anomaly: system.load.1",
		Description: "severity=medium score=0.87 host=my-host",
		Timestamp:   0x0102030405060708,
	}
}

func TestAnomalyEventGoldenBytes(t *testing.T) {
	want := goldenEventBytes()
	if len(want) != 81 {
		t.Fatalf("golden vector length = %d, want 81", len(want))
	}

	encoded, err := goldenEvent().Encode()
	if err != nil {
		t.Fatalf("Encode error = %v", err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("Encode = %x, want %x", encoded, want)
	}

	size, err := goldenEvent().EncodedLen()
	if err != nil {
		t.Fatalf("EncodedLen error = %v", err)
	}
	if size != len(want) {
		t.Fatalf("EncodedLen = %d, want %d", size, len(want))
	}

	decoded, err := DecodeAnomalyEvent(want)
	if err != nil {
		t.Fatalf("DecodeAnomalyEvent error = %v", err)
	}
	if decoded != goldenEvent() {
		t.Fatalf("DecodeAnomalyEvent = %+v, want %+v", decoded, goldenEvent())
	}
}

func TestAnomalyEventsDescriptor(t *testing.T) {
	if AnomalyEventsDescriptor.ID != id8("AAD-EVNT") {
		t.Fatalf("descriptor ID = %q, want %q", AnomalyEventsDescriptor.ID, "AAD-EVNT")
	}
	if AnomalyEventsDescriptor.Version != 1 {
		t.Fatalf("descriptor version = %d, want 1", AnomalyEventsDescriptor.Version)
	}
	if len(AnomalyEventsDescriptor.MessageTypes) != 1 || AnomalyEventsDescriptor.MessageTypes[0] != TypeAnomalyEvent {
		t.Fatalf("descriptor message types = %v, want [%d]", AnomalyEventsDescriptor.MessageTypes, TypeAnomalyEvent)
	}
	if TypeAnomalyEvent != 1 {
		t.Fatalf("TypeAnomalyEvent = %d, want 1", TypeAnomalyEvent)
	}
}

func TestAnomalyEventRoundTripUTF8AndEmptyDescription(t *testing.T) {
	event := AnomalyEvent{
		Title:       "AAD debug trigger: debug.trigger-anomaly.café",
		Description: "",
		Timestamp:   1791536046,
	}
	encoded, err := event.Encode()
	if err != nil {
		t.Fatalf("Encode error = %v", err)
	}
	size, err := event.EncodedLen()
	if err != nil {
		t.Fatalf("EncodedLen error = %v", err)
	}
	if size != len(encoded) {
		t.Fatalf("EncodedLen = %d, encoded = %d", size, len(encoded))
	}
	decoded, err := DecodeAnomalyEvent(encoded)
	if err != nil {
		t.Fatalf("DecodeAnomalyEvent error = %v", err)
	}
	if decoded != event {
		t.Fatalf("DecodeAnomalyEvent = %+v, want %+v", decoded, event)
	}
}

func TestOversizedAnomalyEventIsRejected(t *testing.T) {
	event := AnomalyEvent{Title: "t", Description: strings.Repeat("x", MaxAnomalyEventPayload), Timestamp: 1}
	if _, err := event.Encode(); !errors.Is(err, errEventTooLarge) {
		t.Fatalf("Encode error = %v, want %v", err, errEventTooLarge)
	}
	if _, err := event.EncodedLen(); !errors.Is(err, errEventTooLarge) {
		t.Fatalf("EncodedLen error = %v, want %v", err, errEventTooLarge)
	}

	oversized := make([]byte, MaxAnomalyEventPayload+1)
	if _, err := DecodeAnomalyEvent(oversized); !errors.Is(err, errEventTooLarge) {
		t.Fatalf("DecodeAnomalyEvent error = %v, want %v", err, errEventTooLarge)
	}
}

func TestMalformedAnomalyEventPayloadsAreRejected(t *testing.T) {
	golden := goldenEventBytes()

	truncated := golden[:len(golden)-1]
	if _, err := DecodeAnomalyEvent(truncated); !errors.Is(err, errTruncated) {
		t.Fatalf("truncated payload error = %v, want %v", err, errTruncated)
	}

	trailing := append(append([]byte{}, golden...), 0)
	if _, err := DecodeAnomalyEvent(trailing); !errors.Is(err, errTrailingBytes) {
		t.Fatalf("trailing payload error = %v, want %v", err, errTrailingBytes)
	}

	// A title length prefix that overruns the payload.
	badLength := []byte{255, 255, 255, 255, 'a'}
	if _, err := DecodeAnomalyEvent(badLength); !errors.Is(err, errTruncated) {
		t.Fatalf("bad length error = %v, want %v", err, errTruncated)
	}

	// An invalid UTF-8 title.
	invalidUTF8 := []byte{1, 0, 0, 0, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if _, err := DecodeAnomalyEvent(invalidUTF8); !errors.Is(err, errInvalidUTF8) {
		t.Fatalf("invalid UTF-8 error = %v, want %v", err, errInvalidUTF8)
	}
}

func TestDecodeAnomalyEventRejectsUnknownType(t *testing.T) {
	if _, err := decodeAnomalyEvent(7, []byte("unknown")); err == nil {
		t.Fatal("a record type outside the protocol must be rejected")
	}
}

func TestEventSubscriberReceivesPublishedEvents(t *testing.T) {
	if !Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpoint, err := fitcore.ParseEndpoint("unix:" + newSocketPath(t))
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	// A join does not retry: the publisher owns the endpoint, so a subscriber that
	// arrives first must try again. The consumer does this in its own loop; here it
	// keeps the test from racing the publisher's bind.
	subscribeErr := make(chan error, 1)
	received := make(chan AnomalyEvent, 2)
	go func() {
		config := fitcore.NewSubscriberConfig(endpoint)
		config.SetupTimeout = 5 * time.Second
		deadline := time.Now().Add(30 * time.Second)
		var subscriber *EventSubscriber
		for {
			subscriber, err = SubscribeEventSubscriber(config)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				subscribeErr <- err
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		defer func() { _ = subscriber.Close() }()
		subscribeErr <- nil
		for range 2 {
			event, err := subscriber.Receive()
			if err != nil {
				return
			}
			received <- event
		}
	}()

	publisher, err := fitcore.OpenBroadcastPublisher(fitcore.NewBroadcastConfig(endpoint), AnomalyEventsDescriptor)
	if err != nil {
		t.Fatalf("open publisher: %v", err)
	}
	defer func() { _ = publisher.Close() }()

	want := []AnomalyEvent{
		{Title: "AAD anomaly: system.load.1", Description: "severity medium->high with 3 contributing series", Timestamp: 1_791_536_046},
		{Title: "AAD debug trigger: debug.trigger-anomaly.check", Description: "value=1 host=h tags=[] channel=0", Timestamp: 1_791_536_047},
	}
	// A send with no subscriber waits for one, so it must not block this goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, event := range want {
			payload, err := event.Encode()
			if err != nil {
				return
			}
			publisher.SendBatch([]fitcore.Record{{Kind: TypeAnomalyEvent, Payload: payload}})
		}
	}()

	select {
	case err := <-subscribeErr:
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the subscriber never joined")
	}
	for _, event := range want {
		select {
		case got := <-received:
			if got != event {
				t.Fatalf("received %+v, want %+v", got, event)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("no event arrived before the deadline")
		}
	}
	<-done
}
