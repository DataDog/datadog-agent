// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import "github.com/DataDog/datadog-agent/pkg/util/log"

func loadInitConfigProfiles(initConfigProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	profiles := make(ProfileConfigMap, len(initConfigProfiles))

	for name, profConfig := range initConfigProfiles {
		if profConfig.DefinitionFile != "" {
			profDefinition, err := readProfileDefinition(profConfig.DefinitionFile)
			if err != nil {
				log.Warnf("failed to read profile definition `%s`: %s", name, err)
				continue
			}
			profConfig.Definition = *profDefinition
		}
		profiles[name] = profConfig
	}
	resolvedProfiles, err := loadProfiles(profiles, nil)
	if err != nil {
		return nil, err
	}
	return resolvedProfiles, nil
}
