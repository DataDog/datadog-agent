// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the traceroute component
package mock

import (
	"testing"

	traceroute "github.com/DataDog/datadog-agent/comp/system-probe/traceroute/def"
)

// Mock returns a mock for traceroute component.
func Mock(t *testing.T) traceroute.Component {
	// TODO: Implement the traceroute mock
	return nil
}
