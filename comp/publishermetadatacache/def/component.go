//go:build windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package publishermetadatacache provides a cache for Windows Event Log publisher metadata handles
package publishermetadatacache

import (
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// team: windows-agent

// Component is a cache for EventPublisherMetadataHandle instances.
// It caches handles obtained from EvtOpenPublisherMetadata calls to avoid expensive repeated calls.
type Component interface {
	// Get retrieves a cached EventPublisherMetadataHandle for the given publisher name.
	// If not found in cache, it calls EvtOpenPublisherMetadata and caches the result.
	// Returns the handle and any error encountered.
	Get(publisherName string, event evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error)
}
