// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the daemonchecker component
package mock

import (
	"testing"

	daemonchecker "github.com/DataDog/datadog-agent/comp/updater/daemonchecker/def"
)

type mockDaemonChecker struct{}

func (m *mockDaemonChecker) IsRunning() (bool, error) {
	return true, nil
}

// Mock returns a mock for daemonchecker component.
func Mock(_ *testing.T) daemonchecker.Component {
	return &mockDaemonChecker{}
}
