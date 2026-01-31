// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the oomkill component
package mock

import (
	"testing"

	oomkill "github.com/DataDog/datadog-agent/comp/system-probe/oomkill/def"
)

// Mock returns a mock for oomkill component.
func Mock(_t *testing.T) oomkill.Component {
	// TODO: Implement the oomkill mock
	return nil
}
