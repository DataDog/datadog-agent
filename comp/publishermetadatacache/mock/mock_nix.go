//go:build test && !windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock for the publishermetadatacache component
package mock

import (
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
)

// MockPublisherMetadataCache is a no-op mock for non-Windows platforms
type MockPublisherMetadataCache struct{}

// Mock returns a mock for publishermetadatacache component.
func Mock() publishermetadatacache.Component {
	return &MockPublisherMetadataCache{}
}

