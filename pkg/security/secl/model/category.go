// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// EventCategory category type
type EventCategory int

// Event categories
const (
	// FIMCategory FIM events
	FIMCategory EventCategory = iota
	// ProcessCategory process events
	ProcessCategory
	// KernelCategory Kernel events
	KernelCategory
	// NetworkCategory network events
	NetworkCategory
)

// EventCategoryUserFacing user facing category type
type EventCategoryUserFacing string

const (
	// FIMCategoryUserFacing FIM events
	FIMCategoryUserFacing EventCategoryUserFacing = "File Activity"
	// ProcessCategoryUserFacing process events
	ProcessCategoryUserFacing EventCategoryUserFacing = "Process Activity"
	// KernelCategoryUserFacing Kernel events
	KernelCategoryUserFacing EventCategoryUserFacing = "Kernel Activity"
	// NetworkCategoryUserFacing network events
	NetworkCategoryUserFacing EventCategoryUserFacing = "Network Activity"
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
	// Process
	case
		ExecEventType.String(),
		SignalEventType.String(),
		ExitEventType.String(),
		ForkEventType.String(),
		SyscallsEventType.String(),
		SetrlimitEventType.String():
		return ProcessCategory

	// Kernel
	case
		BPFEventType.String(),
		SELinuxEventType.String(),
		MMapEventType.String(),
		MProtectEventType.String(),
		PTraceEventType.String(),
		UnloadModuleEventType.String(),
		AcceptEventType.String(),
		BindEventType.String(),
		ConnectEventType.String(),
		SysCtlEventType.String():
		return KernelCategory

	// Network
	case
		IMDSEventType.String(),
		RawPacketEventType.String(),
		DNSEventType.String(),
		FullDNSResponseEventType.String(),
		NetworkFlowMonitorEventType.String():
		return NetworkCategory
	}

	return FIMCategory
}

// GetEventTypeCategoryUserFacing returns the category for the given event type
func GetEventTypeCategoryUserFacing(eventType eval.EventType) EventCategoryUserFacing {
	switch eventType {
	case
		BindEventType.String(),
		ConnectEventType.String():
		return NetworkCategoryUserFacing
	}

	switch GetEventTypeCategory(eventType) {
	case FIMCategory:
		return FIMCategoryUserFacing
	case ProcessCategory:
		return ProcessCategoryUserFacing
	case KernelCategory:
		return KernelCategoryUserFacing
	case NetworkCategory:
		return NetworkCategoryUserFacing
	}

	panic("unknown category for given event type")
}

// GetEventTypePerCategory returns the event types per category
func GetEventTypePerCategory(categories ...EventCategory) map[EventCategory][]eval.EventType {
	result := make(map[EventCategory][]eval.EventType)

	var eventTypes []eval.EventType
	var exists bool

	m := &Model{}
	for _, eventType := range m.GetEventTypes() {
		category := GetEventTypeCategory(eventType)
		if len(categories) > 0 && !slices.Contains(categories, category) {
			continue
		}

		if eventTypes, exists = result[category]; exists {
			eventTypes = append(eventTypes, eventType)
		} else {
			eventTypes = []eval.EventType{eventType}
		}
		result[category] = eventTypes
	}

	return result
}
