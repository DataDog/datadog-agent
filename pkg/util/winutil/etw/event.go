// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package etw

/*
#include "etw.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"math"
	"time"

	"golang.org/x/sys/windows"
)

// Event represents a parsed ETW event from an ETL file.
// ProviderID, EventID, and Timestamp are available directly.
// Use EventProperties or GetEventPropertyString for event-specific data.
// The Event is only valid during the ProcessETLFile callback; do not use it after the callback returns.
type Event struct {
	ProviderID  windows.GUID
	EventID     uint16
	Timestamp   time.Time
	eventRecord C.PEVENT_RECORD
}

// EventProperties returns a map of property names to values for this event.
// Uses TDH (Trace Data Helper) to parse the event schema and user data.
func (e *Event) EventProperties() (map[string]interface{}, error) {
	if e.eventRecord == nil {
		return nil, errors.New("event record is nil or no longer valid")
	}
	p, err := newPropertyParser(e.eventRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to parse event properties: %w", err)
	}
	defer p.free()

	props := make(map[string]interface{}, int(p.info.TopLevelPropertyCount))
	for i := 0; i < int(p.info.TopLevelPropertyCount); i++ {
		name := p.getPropertyName(i)
		value, err := p.getPropertyValue(i)
		if err != nil {
			return nil, fmt.Errorf("failed to parse property [%d] %q: %w", i, name, err)
		}
		props[name] = value
	}
	return props, nil
}

// GetPropertyByName retrieves a single property by name using TdhGetProperty.
// Unlike GetPropertyString (which parses all properties sequentially),
// this directly looks up the named property and is resilient to schema
// mismatches in other properties within the same event.
func (e *Event) GetPropertyByName(name string) (string, error) {
	return getPropertyByName(e.eventRecord, name)
}

// GetEventPropertyString returns the string value of a named property.
// GetPropertyString on Event provides the same functionality as a method.
func GetEventPropertyString(e *Event, name string) string {
	props, err := e.EventProperties()
	if err != nil {
		return ""
	}
	if v, ok := props[name]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// GetPropertyString returns the string value of a named property.
// Implements the interface used by logonduration for event property access.
func (e *Event) GetPropertyString(name string) string {
	return GetEventPropertyString(e, name)
}

// GetProviderID returns the event's provider GUID.
func (e *Event) GetProviderID() windows.GUID {
	return e.ProviderID
}

// GetEventID returns the event's ID.
func (e *Event) GetEventID() uint16 {
	return e.EventID
}

// GetTimestamp returns the event's timestamp.
func (e *Event) GetTimestamp() time.Time {
	return e.Timestamp
}

func fileTimeToGo(quadPart C.LONGLONG) time.Time {
	ft := windows.Filetime{
		HighDateTime: uint32(quadPart >> 32),
		LowDateTime:  uint32(quadPart & math.MaxUint32),
	}
	return time.Unix(0, ft.Nanoseconds())
}

func guidFromC(g C.GUID) windows.GUID {
	var data4 [8]byte
	for i := range data4 {
		data4[i] = byte(g.Data4[i])
	}
	return windows.GUID{
		Data1: uint32(g.Data1),
		Data2: uint16(g.Data2),
		Data3: uint16(g.Data3),
		Data4: data4,
	}
}
