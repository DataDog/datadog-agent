// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the diagnose component
package mock

import (
	"testing"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

// Mock returns a mock for diagnose component.
func Mock(_ *testing.T) diagnose.Component {
	// TODO: Implement the diagnose mock
	return nil
}
