// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the ping component
package mock

import (
	"testing"

	ping "github.com/DataDog/datadog-agent/comp/system-probe/ping/def"
)

// Mock returns a mock for ping component.
func Mock(t *testing.T) ping.Component {
	// TODO: Implement the ping mock
	return nil
}
