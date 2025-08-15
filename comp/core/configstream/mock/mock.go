// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the configstream component
package mock

import (
	"testing"

	configstream "github.com/DataDog/datadog-agent/comp/core/configstream/def"
)

// Mock returns a mock for configstream component.
func Mock(t *testing.T) configstream.Component {
	return nil
}
