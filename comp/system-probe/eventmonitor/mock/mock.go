// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the eventmonitor component
package mock

import (
	"testing"

	eventmonitor "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/def"
)

// Mock returns a mock for eventmonitor component.
func Mock(t *testing.T) eventmonitor.Component {
	// TODO: Implement the eventmonitor mock
	return nil
}
