// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the connectivitychecker component
package mock

import (
	"testing"

	connectivitychecker "github.com/DataDog/datadog-agent/comp/connectivitychecker/def"
)

// Mock returns a mock for connectivitychecker component.
func Mock(_ *testing.T) connectivitychecker.Component {
	// TODO: Implement the connectivitychecker mock
	return nil
}
