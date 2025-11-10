// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func loadInitConfigProfiles(rawInitConfigProfiles ProfileConfigMap) (ProfileConfigMap, bool, error) {
	initConfigProfiles := make(ProfileConfigMap, len(rawInitConfigProfiles))

	var haveLegacyInitConfigProfile bool
	for name, profConfig := range rawInitConfigProfiles {
		if profConfig.DefinitionFile != "" {
			profDefinition, isLegacyInitConfigProfile, err := readProfileDefinition(profConfig.DefinitionFile)
			haveLegacyInitConfigProfile = haveLegacyInitConfigProfile || isLegacyInitConfigProfile
			if err != nil {
				log.Warnf("unable to load profile %q: %s", name, err)
				continue
			}
			profConfig.Definition = *profDefinition
		} else {
			isLegacyMetrics := profiledefinition.IsLegacyMetrics(profConfig.Definition.Metrics)
			haveLegacyInitConfigProfile = haveLegacyInitConfigProfile || isLegacyMetrics
		}
		if profConfig.Definition.Name == "" {
			profConfig.Definition.Name = name
		}
		initConfigProfiles[name] = profConfig
	}

	userProfiles, haveLegacyUserProfile := getYamlUserProfiles()
	userProfiles = mergeProfiles(userProfiles, initConfigProfiles)

	defaultProfiles := getYamlDefaultProfiles()
	resolvedProfiles := resolveProfiles(userProfiles, defaultProfiles)

	// When user profiles are from initConfigProfiles
	// only profiles listed in initConfigProfiles are returned
	filteredResolvedProfiles := ProfileConfigMap{}
	for key, val := range resolvedProfiles {
		if _, ok := initConfigProfiles[key]; !ok {
			continue
		}
		filteredResolvedProfiles[key] = val
	}

	return filteredResolvedProfiles, haveLegacyInitConfigProfile || haveLegacyUserProfile, nil
}
