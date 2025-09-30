// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && windows

// Package mock provides a mock for the publishermetadatacache component
package mock

import (
	"testing"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// PublisherMetadataCache is a mock implementation of the publishermetadatacache Component
type publisherMetadataCache struct {
}

// New returns a mock for publishermetadatacache component.
func New(_ testing.TB) publishermetadatacache.Component {
	return &publisherMetadataCache{}
}

// Get implements the Component interface
func (m *publisherMetadataCache) Get(_ string) evtapi.EventPublisherMetadataHandle {
	return evtapi.EventPublisherMetadataHandle(0)
}

// FormatMessage implements the Component interface
func (m *publisherMetadataCache) FormatMessage(_ string, _ evtapi.EventRecordHandle, _ uint) string {
	return ""
}

// Flush implements the Component interface
func (m *publisherMetadataCache) Flush() {
}
