// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && windows

// Package mock provides a mock for the publishermetadatacache component
package mock

import (
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// PublisherMetadataCache is a mock implementation of the publishermetadatacache Component
type PublisherMetadataCache struct{}

// Mock returns a mock for publishermetadatacache component.
func Mock() publishermetadatacache.Component {
	return &PublisherMetadataCache{}
}

// Get implements the Component interface
func (m *PublisherMetadataCache) Get(_ string, _ evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error) {
	return evtapi.EventPublisherMetadataHandle(12345), nil
}
