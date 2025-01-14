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

// GetProfileProvider returns a Provider that knows the on-disk profiles as well as any overrides from the initConfig.
func GetProfileProvider(initConfigProfiles ProfileConfigMap) (Provider, error) {
	profiles, err := loadProfiles(initConfigProfiles)
	if err != nil {
		return nil, err
	}
	return StaticProvider(profiles), nil
}

func loadProfiles(initConfigProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	var profiles ProfileConfigMap
	if len(initConfigProfiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, err := loadInitConfigProfiles(initConfigProfiles)
		if err != nil {
			return nil, fmt.Errorf("failed to load profiles from initConfig: %w", err)
		}
		profiles = customProfiles
	} else {
		defaultProfiles, err := loadYamlProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load yaml profiles: %w", err)
		}
		profiles = defaultProfiles
	}
	for _, profileDef := range profiles {
		profiledefinition.NormalizeMetrics(profileDef.Definition.Metrics)
	}
	return profiles, nil
}

// getProfileForSysObjectID return a profile for a sys object id
func getProfileForSysObjectID(profiles ProfileConfigMap, sysObjectID string) (string, error) {
	tmpSysOidToProfile := map[string]string{}
	var matchedOids []string

	for profile, profConfig := range profiles {
		for _, oidPattern := range profConfig.Definition.SysObjectIDs {
			found, err := filepath.Match(oidPattern, sysObjectID)
			if err != nil {
				log.Debugf("pattern error in profile %q: %v", profile, err)
				continue
			}
			if !found {
				continue
			}
			if prevMatchedProfile, ok := tmpSysOidToProfile[oidPattern]; ok {
				if profiles[prevMatchedProfile].IsUserProfile && !profConfig.IsUserProfile {
					continue
				}
				if profiles[prevMatchedProfile].IsUserProfile == profConfig.IsUserProfile {
					return "", fmt.Errorf("profile %q has the same sysObjectID (%s) as %q", profile, oidPattern, prevMatchedProfile)
				}
			}
			tmpSysOidToProfile[oidPattern] = profile
			matchedOids = append(matchedOids, oidPattern)
		}
	}
	if len(matchedOids) == 0 {
		return "", fmt.Errorf("no profiles found for sysObjectID %q", sysObjectID)
	}
	oid, err := getMostSpecificOid(matchedOids)
	if err != nil {
		return "", fmt.Errorf("failed to get most specific profile for sysObjectID %q, for matched oids %v: %w", sysObjectID, matchedOids, err)
	}
	return tmpSysOidToProfile[oid], nil
}
