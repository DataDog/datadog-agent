// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the updater telemetry component.
package mock

import (
	"testing"

	telemetry "github.com/DataDog/datadog-agent/comp/updater/telemetry/def"
)

type mockTelemetry struct{}

// Mock returns a mock for the updater telemetry component.
func Mock(_ *testing.T) telemetry.Component {
	return &mockTelemetry{}
}
