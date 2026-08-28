// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package profile contains profile related code
package profile

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// GetProfileProvider returns a Provider that knows the on-disk profiles as well as any overrides
// from the initConfig, along with the sorted names of the profiles using the legacy Python syntax.
func GetProfileProvider(initConfigProfiles ProfileConfigMap) (Provider, []string, error) {
	profiles, legacyProfiles, err := loadProfiles(initConfigProfiles)
	if err != nil {
		return nil, nil, err
	}
	return StaticProvider(profiles), legacyProfiles, nil
}

func loadProfiles(initConfigProfiles ProfileConfigMap) (ProfileConfigMap, []string, error) {
	var profiles ProfileConfigMap
	var legacyProfiles []string

	if len(initConfigProfiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, legacyCustomProfiles, err := loadInitConfigProfiles(initConfigProfiles)
		if err != nil {
			return nil, legacyCustomProfiles, fmt.Errorf("failed to load profiles from initConfig: %w", err)
		}
		profiles = customProfiles
		legacyProfiles = legacyCustomProfiles
	} else {
		defaultProfiles, legacyYamlProfiles, err := loadYamlProfiles()
		if err != nil {
			return nil, legacyYamlProfiles, fmt.Errorf("failed to load yaml profiles: %w", err)
		}
		profiles = defaultProfiles
		legacyProfiles = legacyYamlProfiles
	}
	for _, profileDef := range profiles {
		profiledefinition.NormalizeMetrics(profileDef.Definition.Metrics)
	}

	return profiles, legacyProfiles, nil
}

// getProfileForSysObjectID return a profile for a sys object id
func getProfileForSysObjectID(profiles ProfileConfigMap, sysObjectID string) (*ProfileConfig, error) {
	type profileWithKey struct {
		nameKey       string
		profileConfig *ProfileConfig
	}

	tmpSysOidToProfile := map[string]profileWithKey{}
	var matchedOIDs []string

	for profileName, profConfig := range profiles {
		for _, oidPattern := range profConfig.Definition.SysObjectIDs {
			found, err := filepath.Match(oidPattern, sysObjectID)
			if err != nil {
				log.Debugf("pattern error in profile %q: %v", profileName, err)
				continue
			}
			if !found {
				continue
			}
			if prevMatchedProfile, ok := tmpSysOidToProfile[oidPattern]; ok {
				if profiles[prevMatchedProfile.nameKey].IsUserProfile && !profConfig.IsUserProfile {
					continue
				}
				if profiles[prevMatchedProfile.nameKey].IsUserProfile == profConfig.IsUserProfile {
					return nil, fmt.Errorf("profile %q has the same sysObjectID (%s) as %q", profileName, oidPattern,
						prevMatchedProfile.nameKey)
				}
			}
			tmpSysOidToProfile[oidPattern] = profileWithKey{
				nameKey:       profileName,
				profileConfig: &profConfig,
			}
			matchedOIDs = append(matchedOIDs, oidPattern)
		}
	}
	if len(matchedOIDs) == 0 {
		return nil, fmt.Errorf("no profiles found for sysObjectID %q", sysObjectID)
	}
	oid, err := getMostSpecificOid(matchedOIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get most specific profile for sysObjectID %q, for matched oids %v: %w",
			sysObjectID, matchedOIDs, err)
	}
	return tmpSysOidToProfile[oid].profileConfig, nil
}
