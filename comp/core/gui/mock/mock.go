// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the gui component.
package mock

import (
	"testing"

	gui "github.com/DataDog/datadog-agent/comp/core/gui/def"
)

// Mock returns a mock for the gui component.
func Mock(_ *testing.T) gui.Component {
	return struct{}{}
}
