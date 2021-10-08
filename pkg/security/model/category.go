// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// EventCategory category type
type EventCategory = string

// Event categories
const (
	// FIMCategory FIM events
	FIMCategory EventCategory = "fim"
	// RuntimeCategory Process events
	RuntimeCategory EventCategory = "runtime"
)

// GetEventTypeCategory returns the category for the given event type
func GetEventTypeCategory(eventType eval.EventType) EventCategory {
	if eventType == "exec" {
		return RuntimeCategory
	}

	return FIMCategory
}

// GetEventTypePerCategory returns the event types per category
func GetEventTypePerCategory() map[EventCategory][]eval.EventType {
	categories := make(map[EventCategory][]eval.EventType)

	var eventTypes []eval.EventType
	var exists bool

	m := &Model{}
	for _, eventType := range m.GetEventTypes() {
		category := GetEventTypeCategory(eventType)

		if eventTypes, exists = categories[category]; exists {
			eventTypes = append(eventTypes, eventType)
		} else {
			eventTypes = []eval.EventType{eventType}
		}
		categories[category] = eventTypes
	}

	return categories
}
