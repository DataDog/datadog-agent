// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the process component
package mock

import (
	"testing"

	process "github.com/DataDog/datadog-agent/comp/system-probe/process/def"
)

// Mock returns a mock for process component.
func Mock(t *testing.T) process.Component {
	// TODO: Implement the process mock
	return nil
}
