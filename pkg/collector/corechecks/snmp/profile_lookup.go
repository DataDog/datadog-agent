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
		RequestedMetrics:    nil,
		RequestedMetricTags: nil,
		CollectTopology:     false,
		CollectVPN:          false,
	}
	return cfg.BuildProfile(sysObjectID)
}

// GetExtendedProfileNames returns the list of profile names that the given profile extends
// (e.g. ["_base", "_generic-if"]). Uses default on-disk profiles. Returns nil, nil if the
// profile is not found in the provider.
func GetExtendedProfileNames(profileName string) ([]string, error) {
	provider, _, err := profile.GetProfileProvider(profile.ProfileConfigMap{})
	if err != nil {
		return nil, err
	}
	profileConfig := provider.GetProfile(profileName)
	if profileConfig == nil {
		return nil, nil
	}
	return profileConfig.Definition.Extends, nil
}
