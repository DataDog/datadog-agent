// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the compliance component
package mock

import (
	"testing"

	compliance "github.com/DataDog/datadog-agent/comp/system-probe/compliance/def"
)

// Mock returns a mock for compliance component.
func Mock(t *testing.T) compliance.Component {
	// TODO: Implement the compliance mock
	return nil
}
