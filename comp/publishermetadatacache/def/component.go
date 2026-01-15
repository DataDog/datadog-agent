// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package publishermetadatacache provides a cache for Windows Event Log publisher metadata handles
package publishermetadatacache

import (
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// team: windows-products

// Component is a cache for EventPublisherMetadataHandle instances.
// It caches handles obtained from EvtOpenPublisherMetadata calls to avoid expensive repeated calls.
type Component interface {
	// FormatMessage formats an event message using the cached EventPublisherMetadataHandle.
	FormatMessage(publisherName string, event evtapi.EventRecordHandle, flags uint) (string, error)

	// Flush cleans up all cached handles when the component is no longer needed.
	Flush()
}
