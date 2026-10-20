// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package builtins

import "testing"

func TestIsSupportedSystemdUnitType(t *testing.T) {
	for _, unitType := range []string{
		"service",
		"socket",
		"target",
		"device",
		"mount",
		"automount",
		"swap",
		"timer",
		"path",
		"slice",
		"scope",
	} {
		t.Run(unitType, func(t *testing.T) {
			if !IsSupportedSystemdUnitType(unitType) {
				t.Fatalf("expected %q to be supported", unitType)
			}
		})
	}

	for _, unitType := range []string{"", "unit", "Service", ".service", "service/"} {
		t.Run("unsupported_"+unitType, func(t *testing.T) {
			if IsSupportedSystemdUnitType(unitType) {
				t.Fatalf("expected %q to be unsupported", unitType)
			}
		})
	}
}
