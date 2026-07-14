// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && windows

package setup

func stripAssertionIrrelevantRuntimeOverrides(settings map[string]interface{}) map[string]interface{} {
	if dir, ok := settings["fleet_policies_dir"].(string); ok && dir == fleetPoliciesDirFromOverride() {
		delete(settings, "fleet_policies_dir")
	}
	return settings
}
