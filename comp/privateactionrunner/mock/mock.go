// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the privateactionrunner component
package mock

import (
	"testing"

	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
)

// Mock returns a mock for privateactionrunner component.
func Mock(t *testing.T) privateactionrunner.Component {
	// TODO: Implement the privateactionrunner mock
	return nil
}
