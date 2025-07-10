// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the remoteagent component
package mock

import (
	"testing"

	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
)

// Mock returns a mock for remoteagent component.
func Mock(t *testing.T) remoteagent.Component {
	// TODO: Implement the remoteagent mock
	return nil
}
