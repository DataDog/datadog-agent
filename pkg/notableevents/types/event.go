// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package types defines the pure-Go wire contract shared by system-probe and
// the core Agent for macOS notable events.
package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxEventStringBytes bounds each user-derived event string.
	MaxEventStringBytes = 128
	// MaxEventWireSize bounds one serialized event in storage and transport.
	MaxEventWireSize = 16 * 1024
	// MaxCustomDepth bounds nested custom payload containers.
	MaxCustomDepth = 8
	// MaxCustomNodes bounds total custom payload values.
	MaxCustomNodes = 512
	// MaxCustomItems bounds each custom payload container.
	MaxCustomItems = 128

	eventIDPrefix   = "macos-crash-v1:"
	sha256HexLength = 64
)

// Event is the sanitized wire representation of a notable event.
//
// ID is stable across retries and restarts. Event never contains the source
// report filename or an executable path.
type Event struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Custom    map[string]interface{} `json:"custom,omitempty"`
}

// UnmarshalJSON preserves exact JSON numbers and validates the wire contract.
func (e *Event) UnmarshalJSON(data []byte) error {
	type eventAlias Event

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded eventAlias
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONDecoderEOF(decoder); err != nil {
		return err
	}
	event := Event(decoded)
	if err := ValidateEvent(event); err != nil {
		return err
	}
	*e = event
	return nil
}

// ValidateEvent validates one event before persistence or transport.
func ValidateEvent(event Event) error {
	if !IsEventID(event.ID) ||
		event.Timestamp.IsZero() ||
		event.EventType == "" || !isBoundedUTF8String(event.EventType, MaxEventStringBytes) ||
		event.Title == "" || !isBoundedUTF8String(event.Title, MaxEventStringBytes) ||
		event.Message == "" || !isBoundedUTF8String(event.Message, MaxEventStringBytes) {
		return errors.New("invalid notable event fields")
	}
	nodes := 0
	if event.Custom == nil || !validateCustomValue(event.Custom, 0, &nodes) {
		return errors.New("invalid notable event custom payload")
	}
	data, err := json.Marshal(event)
	if err != nil || len(data) > MaxEventWireSize {
		return errors.New("notable event exceeds wire limit")
	}
	return nil
}

// IsEventID reports whether id has the stable macOS crash identifier format.
func IsEventID(id string) bool {
	if !strings.HasPrefix(id, eventIDPrefix) {
		return false
	}
	value := strings.TrimPrefix(id, eventIDPrefix)
	if len(value) != sha256HexLength || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateCustomValue(value interface{}, depth int, nodes *int) bool {
	if depth > MaxCustomDepth {
		return false
	}
	*nodes++
	if *nodes > MaxCustomNodes {
		return false
	}
	switch typed := value.(type) {
	case nil, bool:
		return true
	case json.Number:
		decoder := json.NewDecoder(strings.NewReader(typed.String()))
		decoder.UseNumber()
		var decoded json.Number
		if err := decoder.Decode(&decoded); err != nil || decoded.String() != typed.String() {
			return false
		}
		if err := ensureJSONDecoderEOF(decoder); err != nil {
			return false
		}
		number, err := decoded.Float64()
		return err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
	case float64:
		return !math.IsInf(typed, 0) && !math.IsNaN(typed)
	case float32:
		number := float64(typed)
		return !math.IsInf(number, 0) && !math.IsNaN(number)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case string:
		return isBoundedUTF8String(typed, MaxEventStringBytes)
	case map[string]interface{}:
		if len(typed) > MaxCustomItems {
			return false
		}
		for key, child := range typed {
			if key == "" || !isBoundedUTF8String(key, MaxEventStringBytes) ||
				!validateCustomValue(child, depth+1, nodes) {
				return false
			}
		}
		return true
	case []interface{}:
		if len(typed) > MaxCustomItems {
			return false
		}
		for _, child := range typed {
			if !validateCustomValue(child, depth+1, nodes) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isBoundedUTF8String(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value)
}

func ensureJSONDecoderEOF(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
