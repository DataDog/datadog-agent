// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// EventCategory category type
type EventCategory = string

// Event categories
const (
	// FIMCategory FIM events
	FIMCategory EventCategory = "File Activity"
	// ProcessCategory process events
	ProcessCategory EventCategory = "Process Activity"
	// KernelCategory Kernel events
	KernelCategory EventCategory = "Kernel Activity"
	// NetworkCategory network events
	NetworkCategory EventCategory = "Network Activity"
)

// GetAllCategories returns all categories
func GetAllCategories() []EventCategory {
	return []EventCategory{
		FIMCategory,
		ProcessCategory,
		KernelCategory,
		NetworkCategory,
	}
}

// GetEventTypeCategory returns the category for the given event type
func GetEventTypeCategory(eventType eval.EventType) EventCategory {
	switch eventType {
	case "exec", "signal", "exit", "fork", "anomaly_detection_syscall":
		return ProcessCategory
	case "bpf", "selinux", "mmap", "mprotect", "ptrace", "load_module", "unload_module", "bind":
		// TODO(will): "bind" is in this category because answering "NetworkCategory" would insert a network section in the serializer.
		return KernelCategory
	case "dns":
		return NetworkCategory
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
