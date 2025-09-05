// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the syntheticstestscheduler component
package mock

import (
	"testing"

	syntheticstestscheduler "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/def"
)

// Mock returns a mock for syntheticstestscheduler component.
func Mock(t *testing.T) syntheticstestscheduler.Component {
	// TODO: Implement the syntheticstestscheduler mock
	return nil
}
