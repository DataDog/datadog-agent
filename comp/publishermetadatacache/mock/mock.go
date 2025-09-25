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

// MockPublisherMetadataCache is a mock implementation of the publishermetadatacache Component
type MockPublisherMetadataCache struct {
	GetFunc func(publisherName string, event evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error)
}

// Mock returns a mock for publishermetadatacache component.
func Mock() publishermetadatacache.Component {
	return &MockPublisherMetadataCache{
		GetFunc: func(publisherName string, event evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error) {
			return evtapi.EventPublisherMetadataHandle(12345), nil
		},
	}
}

// Get implements the Component interface
func (m *MockPublisherMetadataCache) Get(publisherName string, event evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error) {
	if m.GetFunc != nil {
		return m.GetFunc(publisherName, event)
	}
	return evtapi.EventPublisherMetadataHandle(12345), nil
}
