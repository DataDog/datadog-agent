// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"testing"
)

// TestEventTypesHaveValidCategories tests that all event types between UnknownEventType (excluded)
// and MaxKernelEventType have a valid category (i.e. non unknown category) defined when calling GetEventTypeCategory.
func TestEventTypesHaveValidCategories(t *testing.T) {
	var eventTypesWithoutCategory []EventType

	// Iterate through all event types between UnknownEventType (excluded) and MaxKernelEventType
	for eventType := UnknownEventType + 1; eventType < MaxKernelEventType; eventType++ {
		// Get the category for this event type
		category := GetEventTypeCategory(eventType.String())

		// Check if the category is unknown (invalid)
		if category == UnknownCategory {
			eventTypesWithoutCategory = append(eventTypesWithoutCategory, eventType)
		}
	}

	// If there are event types without valid categories, fail the test with the list
	if len(eventTypesWithoutCategory) > 0 {
		t.Errorf("The following event types do not have a valid category defined:\n")
		for _, eventType := range eventTypesWithoutCategory {
			t.Errorf("  - %s, aka EventType(%d)\n", eventType.String(), eventType)
		}
		t.Errorf("Total: %d event types without valid categories", len(eventTypesWithoutCategory))
	}
}

// TestEventTypesHaveValidStrings tests that all event types between UnknownEventType (excluded)
// and MaxKernelEventType have a valid string representation (i.e. not "unknown") when calling .String().
func TestEventTypesHaveValidStrings(t *testing.T) {
	var eventTypesWithoutValidString []EventType

	// Iterate through all event types between UnknownEventType (excluded) and MaxKernelEventType
	for eventType := UnknownEventType + 1; eventType < MaxKernelEventType; eventType++ {
		// Get the string representation for this event type
		eventTypeString := eventType.String()

		// Check if the string representation is "unknown" (invalid)
		if eventTypeString == "unknown" {
			eventTypesWithoutValidString = append(eventTypesWithoutValidString, eventType)
		}
	}

	// If there are event types without valid string representations, fail the test with the list
	if len(eventTypesWithoutValidString) > 0 {
		t.Errorf("The following event types do not have a valid string representation defined:\n")
		for _, eventType := range eventTypesWithoutValidString {
			t.Errorf("  - EventType(%d) returns '%s'\n", eventType, eventType.String())
		}
		t.Errorf("Total: %d event types without valid string representations", len(eventTypesWithoutValidString))
	}
}
