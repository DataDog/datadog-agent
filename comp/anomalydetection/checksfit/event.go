// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checksfit

import (
	"encoding/binary"
	"errors"

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
