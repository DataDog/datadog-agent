// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenttelemetryimpl

import (
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
)

// dataLossProfileName is the name of the gated COAT profile toggled by the
// data-loss remote flag. It must match the profile name in defaultProfiles.yaml.
const dataLossProfileName = "data_loss"

// dataLossFlagName is the remote flag that gates the data-loss diagnostic
// profile. The remote flags client lowercases flag names on both ends.
const dataLossFlagName remoteflags.FlagName = "diagnostics_data_loss"

// dataLossFlag is the FlagHandler that turns the "data_loss" diagnostic profile
// on and off in response to the remote flag. Enabling it makes COAT start
// shipping the profile's drop/saturation metrics; disabling it stops shipping.
type dataLossFlag struct {
	a *atel
}

// FlagName returns the remote flag this handler subscribes to.
func (h dataLossFlag) FlagName() remoteflags.FlagName { return dataLossFlagName }

// OnChange enables or disables the data-loss diagnostic profile. The underlying
// toggle is a no-op if the profile is absent from the config, so this never errors.
func (h dataLossFlag) OnChange(value remoteflags.FlagValue) error {
	h.a.setDiagnosticEnabled(dataLossProfileName, bool(value))
	return nil
}

// OnNoConfig disables the profile when Remote Config delivers configs without
// this flag, so a removed flag reverts to the safe (off) default.
func (h dataLossFlag) OnNoConfig() {
	h.a.setDiagnosticEnabled(dataLossProfileName, false)
}

// SafeRecover forces the profile back to the off state. It is idempotent.
func (h dataLossFlag) SafeRecover(_ error, _ remoteflags.FlagValue) {
	h.a.setDiagnosticEnabled(dataLossProfileName, false)
}

// IsHealthy reports whether the last diagnostic collection succeeded. Enabling
// these read-only metrics is low-risk, so this only guards against COAT
// collection itself failing.
func (h dataLossFlag) IsHealthy() bool {
	return h.a.diagnosticCollectionHealthy()
}

// atelFlagSubscriber exposes the agent-telemetry remote flag handlers to the
// Remote Flags component. The handler slice is deliberate so future diagnostic
// categories (e.g. cardinality, resource_cost) can be added here.
type atelFlagSubscriber struct {
	a *atel
}

// Handlers returns the flag handlers owned by the agent-telemetry component.
func (s atelFlagSubscriber) Handlers() []remoteflags.FlagHandler {
	return []remoteflags.FlagHandler{
		dataLossFlag{a: s.a},
	}
}
