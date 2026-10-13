// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// unclearedSharedFields checks an event that has just been zeroed and returns
// the names of any "shared" fields that were not reset to their empty value.
// Shared fields are the ones read for every event, no matter its type (the
// embedded BaseEvent plus Signature, Async, SpanContext, GoLabels and
// NetworkContext). The per-type payload structs (the "...Event" fields) are
// skipped on purpose: the zeroer only clears the payload of the event's own
// type, so the others are expected to still hold data.
func unclearedSharedFields(e *Event) []string {
	want := Event{BaseEvent: BaseEvent{Os: runtime.GOOS}}
	got := reflect.ValueOf(*e)
	exp := reflect.ValueOf(want)
	typ := got.Type()

	var diffs []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.Anonymous && strings.HasSuffix(f.Type.Name(), "Event") {
			continue
		}
		if !reflect.DeepEqual(got.Field(i).Interface(), exp.Field(i).Interface()) {
			diffs = append(diffs, f.Name)
		}
	}
	return diffs
}

// TestEventZeroer_SharedFieldsAlwaysCleared makes sure that, for every event
// type, zeroing an event wipes all the shared fields. It fills an event with
// data, zeroes it, and fails if any shared field still has leftover data. This
// guards against fields leaking from one event into the next reused event.
func TestEventZeroer_SharedFieldsAlwaysCleared(t *testing.T) {
	zero := NewEventZeroer()

	for evtType := UnknownEventType; evtType < MaxAllEventType; evtType++ {
		t.Run(evtType.String(), func(t *testing.T) {
			e := createFullyPopulatedEvent()
			e.Type = uint32(evtType)

			zero(e)

			if diffs := unclearedSharedFields(e); len(diffs) > 0 {
				t.Fatalf("shared fields not cleared for event type %s: %v", evtType, diffs)
			}
		})
	}
}

// TestEventZeroer_ActivePayloadCleared checks that for each event type handled
// by the zeroer's fast path, zeroing the event clears that type's own payload.
// For example, after zeroing a DNS event, the DNS payload should be empty.
func TestEventZeroer_ActivePayloadCleared(t *testing.T) {
	zero := NewEventZeroer()

	tests := []struct {
		evtType EventType
		payload func(*Event) any
		want    any
	}{
		{PrCtlEventType, func(e *Event) any { return e.PrCtl }, PrCtlEvent{}},
		{FileOpenEventType, func(e *Event) any { return e.Open }, OpenEvent{}},
		{SetSockOptEventType, func(e *Event) any { return e.SetSockOpt }, SetSockOptEvent{}},
		{ArgsEnvsEventType, func(e *Event) any { return e.ArgsEnvs }, ArgsEnvsEvent{}},
		{ConnectEventType, func(e *Event) any { return e.Connect }, ConnectEvent{}},
		{MMapEventType, func(e *Event) any { return e.MMap }, MMapEvent{}},
		{DNSEventType, func(e *Event) any { return e.DNS }, DNSEvent{}},
		{FileUnlinkEventType, func(e *Event) any { return e.Unlink }, UnlinkEvent{}},
		{AcceptEventType, func(e *Event) any { return e.Accept }, AcceptEvent{}},
	}

	for _, tc := range tests {
		t.Run(tc.evtType.String(), func(t *testing.T) {
			e := createFullyPopulatedEvent()
			e.Type = uint32(tc.evtType)

			zero(e)

			assert.Equal(t, tc.want, tc.payload(e))
		})
	}
}
