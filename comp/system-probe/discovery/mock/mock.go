// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the discovery component
package mock

import (
	"testing"

	discovery "github.com/DataDog/datadog-agent/comp/system-probe/discovery/def"
)

// Mock returns a mock for discovery component.
func Mock(t *testing.T) discovery.Component {
	// TODO: Implement the discovery mock
	return nil
}
