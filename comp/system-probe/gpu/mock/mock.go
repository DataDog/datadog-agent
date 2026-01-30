// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the gpu component
package mock

import (
	"testing"

	gpu "github.com/DataDog/datadog-agent/comp/system-probe/gpu/def"
)

// Mock returns a mock for gpu component.
func Mock(t *testing.T) gpu.Component {
	// TODO: Implement the gpu mock
	return nil
}
