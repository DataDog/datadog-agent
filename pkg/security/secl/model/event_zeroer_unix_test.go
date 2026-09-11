// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

// unclearedSharedFields returns the names of the cross-cutting Event fields that
// still differ from a freshly zeroed event. The per-event-type payload structs
// (the `…Event` union fields) are excluded: the fast path in NewEventZeroer
// intentionally leaves them untouched, since only the payload of the event's own
// type has been written, and that one is cleared by its own switch branch.
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

func TestEventZeroer_SharedFieldsAlwaysCleared(t *testing.T) {
	zero := NewEventZeroer()

	for evtType := UnknownEventType; evtType < MaxAllEventType; evtType++ {
		t.Run(evtType.String(), func(t *testing.T) {
			e := createFullyPopulatedEvent()
			e.Signature = "leaked-signature"
			e.GoLabels = GoLabelsContext{ID: 42, Resolved: true}
			e.Type = uint32(evtType)

			zero(e)

			if diffs := unclearedSharedFields(e); len(diffs) > 0 {
				t.Fatalf("shared fields not cleared for event type %s: %v", evtType, diffs)
			}
		})
	}
}

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
