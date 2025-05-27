// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the installerexec component
package mock

import (
	"testing"

	installerexec "github.com/DataDog/datadog-agent/comp/updater/installerexec/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
)

// Mock returns a mock for installerexec component.
func Mock(_ *testing.T) installerexec.Component {
	return commands.NewInstallerMock()
}
