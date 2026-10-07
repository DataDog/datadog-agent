// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"slices"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

// InfraModeTagKey is the tag key that marks a payload with the infrastructure
// mode of the Agent that produced it. It differs from the `infrastructure_mode`
// setting name on purpose: the setting name is not a tag key.
const InfraModeTagKey = "infra_mode"

// markedInfraModes lists the modes whose payloads carry the mark.
//
// This is an allowlist rather than a `mode != full` test: the mark is a contract
// with backend consumers, so a mode added later carries no mark until it has a
// named consumer and an entry here.
var markedInfraModes = []string{
	constants.InfraModeCloudCostOnly,
	constants.InfraModeEndUserDevice,
}

// ResolveInfrastructureMode returns the validated `infrastructure_mode` of the
// Agent. A value outside the declared set resolves to full, so that a typo
// cannot produce a mark that no consumer expects. The invalid value itself is
// reported once when the configuration loads, so this does not log.
func ResolveInfrastructureMode(c pkgconfigmodel.Reader) string {
	mode := c.GetString("infrastructure_mode")
	if !constants.IsKnownInfraMode(mode) {
		return constants.InfraModeFull
	}
	return mode
}

// MarkedInfraMode returns the `infra_mode` tag value for the Agent, or the empty
// string when the resolved mode carries no mark.
//
// The value describes the Agent process and not an entity, so it is stored under
// a dedicated Tagger entity and appended where a payload the Agent itself
// produces is serialized.
//
// It must never reach a metric sample. The metrics intake renames series tagged
// `infra_mode:cloud_cost_only` into the `dd.cloud_cost` namespace, so a mark that
// leaks onto a customer custom metric removes that metric from dashboards,
// monitors and metering.
func MarkedInfraMode(c pkgconfigmodel.Reader) string {
	mode := ResolveInfrastructureMode(c)
	if !slices.Contains(markedInfraModes, mode) {
		return ""
	}
	return mode
}
