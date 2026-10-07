// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package constants

import "slices"

// Values accepted by the `infrastructure_mode` setting. They mirror the enum
// documented on the setting in the core schema.
const (
	// InfraModeFull is the default server and host monitoring behavior.
	InfraModeFull = "full"
	// InfraModeBasic collects host-level metrics only.
	InfraModeBasic = "basic"
	// InfraModeEndUserDevice is tuned for end-user-device laptops and desktops.
	InfraModeEndUserDevice = "end_user_device"
	// InfraModeCloudCostOnly serves a customer who pays for Cloud Cost Management only.
	InfraModeCloudCostOnly = "cloud_cost_only"
	// InfraModeNone disables infrastructure checks.
	InfraModeNone = "none"
)

// KnownInfraModes lists every declared value of `infrastructure_mode`.
var KnownInfraModes = []string{
	InfraModeFull,
	InfraModeBasic,
	InfraModeEndUserDevice,
	InfraModeCloudCostOnly,
	InfraModeNone,
}

// IsKnownInfraMode reports whether mode is a declared `infrastructure_mode` value.
// An empty value is not a mode: callers treat it as unset and fall back to
// InfraModeFull without reporting it.
func IsKnownInfraMode(mode string) bool {
	return slices.Contains(KnownInfraModes, mode)
}
