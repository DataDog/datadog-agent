// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import "github.com/DataDog/datadog-agent/pkg/util/log"

func loadInitConfigProfiles(rawInitConfigProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	initConfigProfiles := make(ProfileConfigMap, len(rawInitConfigProfiles))

	for name, profConfig := range rawInitConfigProfiles {
		if profConfig.DefinitionFile != "" {
			profDefinition, err := readProfileDefinition(profConfig.DefinitionFile)
			if err != nil {
				log.Warnf("unable to load profile %q: %s", name, err)
				continue
			}
			profConfig.Definition = *profDefinition
		}
		initConfigProfiles[name] = profConfig
	}

	userProfiles := mergeProfiles(getYamlUserProfiles(), initConfigProfiles)
	resolvedProfiles, err := resolveProfiles(userProfiles, getYamlDefaultProfiles())
	if err != nil {
		return nil, err
	}

	// When user profiles are from initConfigProfiles
	// only profiles listed in initConfigProfiles are returned
	filteredResolvedProfiles := ProfileConfigMap{}
	for key, val := range resolvedProfiles {
		if _, ok := initConfigProfiles[key]; !ok {
			continue
		}
		filteredResolvedProfiles[key] = val
	}
	return filteredResolvedProfiles, nil
}
