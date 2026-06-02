// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test && windows

// Package mock provides a mock for the agentcrashdetect component.
package mock

import (
	"testing"

	agentcrashdetect "github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect/def"
)

// Mock returns a mock for the agentcrashdetect component.
func Mock(_ *testing.T) agentcrashdetect.Component {
	return struct{}{}
}
