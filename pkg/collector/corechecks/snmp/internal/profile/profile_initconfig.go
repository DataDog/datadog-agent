// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"expvar"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// loadInitConfigProfiles returns the profiles declared in the check init config, resolved
// against the on-disk profiles, along with the sorted names of the profiles using the
// legacy Python syntax.
func loadInitConfigProfiles(rawInitConfigProfiles ProfileConfigMap) (ProfileConfigMap, []string, error) {
	initConfigProfiles := make(ProfileConfigMap, len(rawInitConfigProfiles))

	var legacyProfiles []string
	for name, profConfig := range rawInitConfigProfiles {
		if profConfig.DefinitionFile != "" {
			profDefinition, isLegacyInitConfigProfile, err := readProfileDefinition(profConfig.DefinitionFile)
			if isLegacyInitConfigProfile {
				legacyProfiles = append(legacyProfiles, name)
			}
			if err != nil {
				log.Warnf("unable to load profile %q: %s", name, err)
				errMsg := err.Error()
				profileExpVar.Set(name, expvar.Func(func() interface{} {
					return errMsg
				}))
				continue
			}
			profConfig.Definition = *profDefinition
		} else if profiledefinition.IsLegacyMetrics(profConfig.Definition.Metrics) {
			legacyProfiles = append(legacyProfiles, name)
		}
		if profConfig.Definition.Name == "" {
			profConfig.Definition.Name = name
		}
		initConfigProfiles[name] = profConfig
	}

	userProfiles, legacyUserProfiles := getYamlUserProfiles()
	legacyProfiles = append(legacyProfiles, legacyUserProfiles...)
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

	slices.Sort(legacyProfiles)
	return filteredResolvedProfiles, slices.Compact(legacyProfiles), nil
}
