// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import "github.com/DataDog/datadog-agent/pkg/util/log"

func loadProfilesForInitConfig(pConfig ProfileConfigMap) (ProfileConfigMap, error) {
	profiles := make(ProfileConfigMap, len(pConfig))

	for name, profConfig := range pConfig {
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
	resolvedProfiles, err := loadProfilesV3(profiles)
	if err != nil {
		return nil, err
	}
	return resolvedProfiles, nil
}
