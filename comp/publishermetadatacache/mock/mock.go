// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the publishermetadatacache component
package mock

import (
	"testing"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
)

// Mock returns a mock for publishermetadatacache component.
func Mock(t *testing.T) publishermetadatacache.Component {
	// TODO: Implement the publishermetadatacache mock
	return nil
}
