// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checksfit

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
)

// Anomaly event protocol constants. This is a separate application protocol
// from DDCHECKS: the isolated anomaly detection process publishes events on a
// FIT broadcast channel, and every subscriber receives each event.
const (
	// AnomalyEventsProtocolID is the eight-byte application identity.
	AnomalyEventsProtocolID = "AAD-EVNT"
	// AnomalyEventsProtocolVersion is the current protocol version.
	AnomalyEventsProtocolVersion uint32 = 1
	// TypeAnomalyEvent is the record type of an anomaly event payload.
	TypeAnomalyEvent uint32 = 1
)

// MaxAnomalyEventPayload is the largest encoded event either side accepts. The
// transport allows much more, but an event carries a title and a description;
// anything past this bound is a producer bug, so both sides reject it early.
const MaxAnomalyEventPayload = 64 * 1024

var errEventTooLarge = errors.New("anomaly event exceeds the 64 KiB payload cap")

// AnomalyEventsDescriptor identifies the anomaly event protocol during setup.
// Its identity, version, and record types must stay in lockstep with the Rust
// publisher in the checks-protocol crate.
var AnomalyEventsDescriptor = fitcore.ProtocolDescriptor{
	ID:           id8(AnomalyEventsProtocolID),
	Version:      AnomalyEventsProtocolVersion,
	MessageTypes: []uint32{TypeAnomalyEvent},
}

// AnomalyEvent is the type 1 payload of the AAD-EVNT protocol: one anomaly
// notification from the isolated anomaly detection process.
type AnomalyEvent struct {
	// Title is the short human-readable summary.
	Title string
	// Description carries the details, and may be empty.
	Description string
	// Timestamp is the Unix timestamp in seconds of the anomaly itself, not of
	// the publication.
	Timestamp uint64
}

// EncodedLen reports the exact payload length of the encoded event.
func (e AnomalyEvent) EncodedLen() (int, error) {
	titleSize, err := stringSize(e.Title)
	if err != nil {
		return 0, err
	}
	descriptionSize, err := stringSize(e.Description)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, part := range []int{titleSize, descriptionSize, 8} {
		total += part
		if total > MaxAnomalyEventPayload {
			return 0, errEventTooLarge
		}
	}
	return total, nil
}

// Encode serializes the event in the field order of the AAD-EVNT v1 contract:
// `string title`, `string description`, `u64 timestamp`, all little-endian with
// u32 string lengths.
func (e AnomalyEvent) Encode() ([]byte, error) {
	size, err := e.EncodedLen()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, size)
	out = appendString(out, e.Title)
	out = appendString(out, e.Description)
	out = binary.LittleEndian.AppendUint64(out, e.Timestamp)
	return out, nil
}

// DecodeAnomalyEvent parses a type 1 payload, rejecting truncated payloads,
// mismatched lengths, trailing bytes, and invalid UTF-8.
func DecodeAnomalyEvent(b []byte) (AnomalyEvent, error) {
	if len(b) > MaxAnomalyEventPayload {
		return AnomalyEvent{}, errEventTooLarge
	}
	reader := payloadReader{remaining: b}
	out := AnomalyEvent{}
	var err error
	if out.Title, err = reader.string(); err != nil {
		return AnomalyEvent{}, err
	}
	if out.Description, err = reader.string(); err != nil {
		return AnomalyEvent{}, err
	}
	if out.Timestamp, err = reader.u64(); err != nil {
		return AnomalyEvent{}, err
	}
	if len(reader.remaining) != 0 {
		return AnomalyEvent{}, errTrailingBytes
	}
	return out, nil
}

// EventSubscriber is an AAD-EVNT typed broadcast subscriber handle.
type EventSubscriber struct {
	inner *fitcore.Subscription
}

// SubscribeEventSubscriber joins the publisher's broadcast ring using the
// AAD-EVNT descriptor. A late subscriber starts at its activation boundary and
// receives only events published after this call returns.
func SubscribeEventSubscriber(config fitcore.SubscriberConfig) (*EventSubscriber, error) {
	inner, err := fitcore.Subscribe(config, AnomalyEventsDescriptor)
	if err != nil {
		return nil, err
	}
	return &EventSubscriber{inner: inner}, nil
}

// SubscribeEventSubscriberContext joins while allowing the local supervisor to
// cancel the handshake through the context.
func SubscribeEventSubscriberContext(ctx context.Context, config fitcore.SubscriberConfig) (*EventSubscriber, error) {
	inner, err := fitcore.SubscribeContext(ctx, config, AnomalyEventsDescriptor)
	if err != nil {
		return nil, err
	}
	return &EventSubscriber{inner: inner}, nil
}

// SessionID reports the publisher session this subscription belongs to.
func (s *EventSubscriber) SessionID() uint64 { return s.inner.SessionID() }

// SlotID reports the subscriber slot this subscription holds.
func (s *EventSubscriber) SlotID() uint32 { return s.inner.SlotID() }

// Close releases the shared mapping.
func (s *EventSubscriber) Close() error { return s.inner.Close() }

// Unsubscribe releases the slot so it stops pinning publication. After a
// successful call the slot is free, and calling it again is a no-op.
func (s *EventSubscriber) Unsubscribe() error { return s.inner.Unsubscribe() }

// Receive waits for one event.
func (s *EventSubscriber) Receive() (AnomalyEvent, error) {
	kind, payload, err := s.inner.Receive()
	if err != nil {
		return AnomalyEvent{}, err
	}
	return decodeAnomalyEvent(kind, payload)
}

// ReceiveContext waits for one event, or returns an error wrapping
// fitcore.ErrCancelled after local cancellation.
func (s *EventSubscriber) ReceiveContext(ctx context.Context) (AnomalyEvent, error) {
	kind, payload, err := s.inner.ReceiveContext(ctx)
	if err != nil {
		return AnomalyEvent{}, err
	}
	return decodeAnomalyEvent(kind, payload)
}

func decodeAnomalyEvent(kind uint32, payload []byte) (AnomalyEvent, error) {
	if kind != TypeAnomalyEvent {
		return AnomalyEvent{}, fmt.Errorf("unexpected AAD-EVNT record type %d, want %d", kind, TypeAnomalyEvent)
	}
	return DecodeAnomalyEvent(payload)
}
