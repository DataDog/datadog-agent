// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && windows

package mock

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// SetFleetPoliciesDirFromRegistry makes the configurations read dir as the fleet_policies_dir registry value until the
// end of the test, as on a host where a Fleet Automation policy is configured, without writing the registry.
func SetFleetPoliciesDirFromRegistry(t testing.TB, dir string) {
	setup.SetFleetPoliciesDirFromRegistry(t, dir)
}
