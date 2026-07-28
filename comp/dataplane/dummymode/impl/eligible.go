// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package dummymodeimpl

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	DataPlaneEnabled = "data_plane.enabled"
	DataPlaneDummyMode = "data_plane.dummy_mode"
)

// isEligible reports whether the Agent Data Plane should be run in dummy mode.
//
// Dummy mode is a pre-flight check: it starts ADP for a short window, in an isolated fashion,
// so that environment-specific startup problems can be reported back to Datadog prior to ADP
// being released as GA. It therefore only runs when `data_plane.enabled` is `false` _and_
// `false` is coming from the default value.
//
// We additionally evaluate a dummy mode-specific setting -- `data_plane.dummy_mode` -- which
// provides a means to disable dummy mode explicitly if dummy mode itself manages to surface
// any customer-visible errors.
//
// Overall, this ensures that we don't run ADP at all when the operator explicitly indicates that
// ADP should be disabled (`data_plane.enabled: false`) or when it's enabled, since we'd be running
// a second ADP process unnecessarily.
//
// Note that the platform gate in pkg/config/setup locks `data_plane.enabled` to `false` via
// SourceAgentRuntime where ADP cannot run at all -- unsupported platforms, and Windows without
// process_manager.enabled -- which makes this return false there too. That is what we want:
// there is nothing to pre-flight in an environment ADP could never run in.
func isEligible(config pkgconfigmodel.Reader) bool {
	if !config.GetBool(DataPlaneDummyMode) {
		return false
	}

	dataPlaneEnabled := config.GetBool(DataPlaneEnabled)
	dataPlaneEnabledSetViaDefault := config.GetSource(DataPlaneEnabled) == pkgconfigmodel.SourceDefault

	return !dataPlaneEnabled && dataPlaneEnabledSetViaDefault
}
