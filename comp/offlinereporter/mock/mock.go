// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a no-op offlinereporter component for use in tests.
package mock

import (
	"testing"

	offlinereporter "github.com/DataDog/datadog-agent/comp/offlinereporter/def"
)

// Mock returns a no-op offlinereporter component.
func Mock(_ *testing.T) offlinereporter.Component {
	return &mockOfflineReporter{}
}

type mockOfflineReporter struct{}

func (m *mockOfflineReporter) SendOfflineDuration(_ string, _ []string) {}
