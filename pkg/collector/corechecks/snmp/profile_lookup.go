// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// defaultRequestedMetrics mirrors what the agent effectively collects for system info
var defaultRequestedMetrics = []profiledefinition.MetricsConfig{
	{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "sysUpTimeInstance"}}, // sysUpTime
	{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr"}},
	{Symbol: profiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.2", Name: "sysObjectID"}},
}

// BuildProfileForSysObjectID loads default SNMP profiles and returns the profile definition
// that matches the given sysObjectID, or an empty profile and an error if none match.
func BuildProfileForSysObjectID(sysObjectID string) (profiledefinition.ProfileDefinition, error) {
	provider, _, err := profile.GetProfileProvider(profile.ProfileConfigMap{})
	if err != nil {
		return profiledefinition.ProfileDefinition{}, err
	}
	cfg := &checkconfig.CheckConfig{
		ProfileProvider:     provider,
		ProfileName:         checkconfig.ProfileNameAuto,
		RequestedMetrics:    defaultRequestedMetrics,
		RequestedMetricTags: nil,
		CollectTopology:     false,
		CollectVPN:          false,
	}
	return cfg.BuildProfile(sysObjectID)
}

// GetProfileDefinition returns the profile definition for the given profile name (e.g. "_base", "dell").
func GetProfileDefinition(profileName string) (profiledefinition.ProfileDefinition, error) {
	provider, _, err := profile.GetProfileProvider(profile.ProfileConfigMap{})
	if err != nil {
		return profiledefinition.ProfileDefinition{}, err
	}
	profileConfig := provider.GetProfile(profileName)
	if profileConfig == nil {
		return profiledefinition.ProfileDefinition{}, nil
	}
	return profileConfig.Definition, nil
}
