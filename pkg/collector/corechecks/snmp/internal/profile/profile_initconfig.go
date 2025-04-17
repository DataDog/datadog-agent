// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import "github.com/DataDog/datadog-agent/pkg/util/log"

func loadInitConfigProfiles(rawInitConfigProfiles ProfileConfigMap) (ProfileConfigMap, bool, error) {
	initConfigProfiles := make(ProfileConfigMap, len(rawInitConfigProfiles))
	haveLegacyProfile := false

	for name, profConfig := range rawInitConfigProfiles {
		if profConfig.DefinitionFile != "" {
			profDefinition, haveLegacyInitConfigProfile, err := readProfileDefinition(profConfig.DefinitionFile)
			haveLegacyProfile = haveLegacyInitConfigProfile || haveLegacyProfile
			if err != nil {
				log.Warnf("unable to load profile %q: %s", name, err)
				continue
			}
			profConfig.Definition = *profDefinition
		}
		if profConfig.Definition.Name == "" {
			profConfig.Definition.Name = name
		}
		initConfigProfiles[name] = profConfig
	}

	userProfiles, haveLegacyUserProfile := getYamlUserProfiles()
	userProfiles = mergeProfiles(userProfiles, initConfigProfiles)

	defaultProfiles, haveLegacyDefaultProfile := getYamlDefaultProfiles()
	resolvedProfiles, haveLegacyResolvedProfile := resolveProfiles(userProfiles, defaultProfiles)

	// When user profiles are from initConfigProfiles
	// only profiles listed in initConfigProfiles are returned
	filteredResolvedProfiles := ProfileConfigMap{}
	for key, val := range resolvedProfiles {
		if _, ok := initConfigProfiles[key]; !ok {
			continue
		}
		filteredResolvedProfiles[key] = val
	}

	haveLegacyProfile = haveLegacyUserProfile || haveLegacyDefaultProfile || haveLegacyResolvedProfile || haveLegacyProfile
	return filteredResolvedProfiles, haveLegacyProfile, nil
}
