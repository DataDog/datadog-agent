// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package types

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventUnmarshalJSONPreservesCustomNumbers(t *testing.T) {
	const exactInteger = "9007199254740993"
	data := []byte(`{
		"id":"macos-crash-v1:` + strings.Repeat("a", 64) + `",
		"timestamp":"2026-07-22T12:00:00Z",
		"event_type":"Application crash",
		"title":"Application crash: Test",
		"message":"An application crashed unexpectedly",
		"custom":{"value":` + exactInteger + `}
	}`)

	var event Event
	require.NoError(t, json.Unmarshal(data, &event))
	number, ok := event.Custom["value"].(json.Number)
	require.True(t, ok)
	assert.Equal(t, json.Number(exactInteger), number)
}

func TestValidateEventRejectsSemanticCorruption(t *testing.T) {
	valid := validEvent()
	tests := []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "invalid id", mutate: func(event *Event) { event.ID = "event" }},
		{name: "zero timestamp", mutate: func(event *Event) { event.Timestamp = time.Time{} }},
		{name: "empty title", mutate: func(event *Event) { event.Title = "" }},
		{name: "oversized message", mutate: func(event *Event) { event.Message = strings.Repeat("x", MaxEventStringBytes+1) }},
		{name: "nil custom payload", mutate: func(event *Event) { event.Custom = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.mutate(&event)
			require.Error(t, ValidateEvent(event))
		})
	}
}

func TestIsEventIDAcceptsEveryKnownPrefix(t *testing.T) {
	hexTail := strings.Repeat("a", sha256HexLength)

	for _, prefix := range []string{"macos-crash-v1:", "macos-shutdown-v1:"} {
		assert.True(t, IsEventID(prefix+hexTail), "%s should be accepted", prefix)
	}

	for _, id := range []string{
		"macos-thermal-v1:" + hexTail,
		"macos-crash-v2:" + hexTail,
		hexTail,
		"",
		"macos-crash-v1:",
		"macos-shutdown-v1:" + strings.Repeat("a", sha256HexLength-1),
		"macos-shutdown-v1:" + strings.Repeat("a", sha256HexLength+1),
		"macos-shutdown-v1:" + strings.Repeat("A", sha256HexLength),
		"macos-shutdown-v1:" + strings.Repeat("z", sha256HexLength),
	} {
		assert.False(t, IsEventID(id), "%q should be rejected", id)
	}
}

func TestValidateEventAcceptsShutdownID(t *testing.T) {
	event := validEvent()
	event.ID = "macos-shutdown-v1:" + strings.Repeat("b", sha256HexLength)
	event.EventType = "System shutdown fault"
	event.Title = "macOS overheated shutdown"
	event.Message = "The previous shutdown was caused by a thermal fault"

	require.NoError(t, ValidateEvent(event))
}

func TestValidateCustomValueNumbers(t *testing.T) {
	for _, value := range []interface{}{
		json.Number("9007199254740993"),
		json.Number("-1.25e+3"),
		float64(1.5),
		float32(1.5),
		int64(11),
		uint64(math.MaxUint64),
	} {
		nodes := 0
		assert.True(t, validateCustomValue(value, 0, &nodes), "%v should be valid", value)
	}

	for _, value := range []interface{}{
		json.Number("01"),
		json.Number("+1"),
		json.Number("NaN"),
		json.Number("Infinity"),
		json.Number("1e400"),
		math.Inf(1),
		math.NaN(),
		float32(math.Inf(1)),
		float32(math.NaN()),
	} {
		nodes := 0
		assert.False(t, validateCustomValue(value, 0, &nodes), "%v should be invalid", value)
	}
}

func validEvent() Event {
	return Event{
		ID:        "macos-crash-v1:" + strings.Repeat("a", 64),
		Timestamp: time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC),
		EventType: "Application crash",
		Title:     "Application crash: Test",
		Message:   "An application crashed unexpectedly",
		Custom:    map[string]interface{}{"scope": "system"},
	}
}
