// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the diagnoseinventory component
package mock

import (
	"testing"

	diagnoseinventory "github.com/DataDog/datadog-agent/comp/diagnoseinventory/def"
)

// Mock returns a mock for diagnoseinventory component.
func Mock(_ *testing.T) diagnoseinventory.Component {
	// TODO: Implement the diagnoseinventory mock
	return nil
}
