// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the ndmsyslogs component
package mock

import (
	"testing"

	ndmsyslogs "github.com/DataDog/datadog-agent/comp/ndmsyslogs/def"
)

// Mock returns a mock for ndmsyslogs component.
func Mock(t *testing.T) ndmsyslogs.Component {
	// TODO: Implement the ndmsyslogs mock
	return nil
}
