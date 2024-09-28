// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the resourcetracker component
package mock

import (
	"testing"

	resourcetracker "github.com/DataDog/datadog-agent/comp/agent/resourcetracker/def"
)

// Mock returns a mock for resourcetracker component.
func Mock(t *testing.T) resourcetracker.Component {
	return struct{}{}
}
