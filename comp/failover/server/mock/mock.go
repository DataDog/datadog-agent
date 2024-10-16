// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the server component
package mock

import (
	"testing"

	server "github.com/DataDog/datadog-agent/comp/failover/server/def"
)

// Mock returns a mock for server component.
func Mock(t *testing.T) server.Component {
	// TODO: Implement the server mock
	return nil
}
