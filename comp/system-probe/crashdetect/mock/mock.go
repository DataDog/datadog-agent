// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the crashdetect component
package mock

import (
	"testing"

	crashdetect "github.com/DataDog/datadog-agent/comp/system-probe/crashdetect/def"
)

// Mock returns a mock for crashdetect component.
func Mock(_t *testing.T) crashdetect.Component {
	// TODO: Implement the crashdetect mock
	return nil
}
