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

// UnknownCategory for everything without a clear category
var UnknownCategory = EventCategory(-1)

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
	// UnknownCategoryUserFacing used for events without a clear category
	UnknownCategoryUserFacing EventCategoryUserFacing = "Unknown Activity"
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
		ForkEventType.String(),
		SetuidEventType.String(),
		SetgidEventType.String(),
		CapsetEventType.String(),
		SignalEventType.String(),
		ExitEventType.String(),
		SetrlimitEventType.String(),
		CapabilitiesEventType.String(),
		SyscallsEventType.String(),
		LoginUIDWriteEventType.String(),
		PrCtlEventType.String(),
		ArgsEnvsEventType.String():
		return ProcessCategory

	// Kernel
	case
		SELinuxEventType.String(),
		BPFEventType.String(),
		PTraceEventType.String(),
		MMapEventType.String(),
		MProtectEventType.String(),
		LoadModuleEventType.String(),
		UnloadModuleEventType.String(),
		SysCtlEventType.String(),
		CgroupWriteEventType.String(),
		CgroupTracingEventType.String(),
		UnshareMountNsEventType.String(),
		OnDemandEventType.String():
		return KernelCategory

	// Network
	case
		BindEventType.String(),
		ConnectEventType.String(),
		AcceptEventType.String(),
		SetSockOptEventType.String(),
		DNSEventType.String(),
		FullDNSResponseEventType.String(),
		ShortDNSResponseEventType.String(),
		IMDSEventType.String(),
		RawPacketFilterEventType.String(),
		RawPacketActionEventType.String(),
		NetworkFlowMonitorEventType.String(),
		NetDeviceEventType.String(),
		VethPairEventType.String(),
		VethPairNsEventType.String():
		return NetworkCategory

	// FIM
	case
		FileChmodEventType.String(),
		FileChownEventType.String(),
		FileOpenEventType.String(),
		FileMkdirEventType.String(),
		FileRmdirEventType.String(),
		FileRenameEventType.String(),
		FileUnlinkEventType.String(),
		FileUtimesEventType.String(),
		FileLinkEventType.String(),
		FileSetXAttrEventType.String(),
		FileRemoveXAttrEventType.String(),
		SpliceEventType.String(),
		FileMountEventType.String(),
		FileChdirEventType.String(),
		FileUmountEventType.String(),
		InvalidateDentryEventType.String(),
		MountReleasedEventType.String(),
		StatEventType.String(),
		FileFsmountEventType.String(),
		FileOpenTreeEventType.String():
		return FIMCategory
	}

	return UnknownCategory
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
