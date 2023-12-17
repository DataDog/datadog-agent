// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package profile contains profile related code
package profile

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// GetProfiles returns profiles depending on various sources:
//   - init config profiles
//   - yaml profiles
//   - downloaded json gzip profiles
//   - remote config profiles
func GetProfiles(initConfigProfiles ProfileConfigMap) (ProfileConfigMap, error) {
	var profiles ProfileConfigMap

	// TODO: Use a Profile Manager to handle profiles state?
	//       It can include loops to check for new RC Profiles at regular interval.

	if len(initConfigProfiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, err := loadInitConfigProfiles(initConfigProfiles)
		if err != nil {
			return nil, fmt.Errorf("failed to load initConfig profiles: %s", err)
		}
		profiles = customProfiles
	} else if pkgconfig.Datadog.GetBool("remote_configuration.snmp_profiles.enabled") {
		log.Info("Loading REMOTE CONFIG profiles")
		time.Sleep(10 * time.Second) // TODO: Temp Hack to wait for remote config profiles to be available
		defaultProfiles, err := loadRemoteConfigProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load remote config profiles: %s", err)
		}
		log.Infof("defaultProfiles count: %d", len(defaultProfiles))
		profiles = defaultProfiles
	} else if profileBundleFileExist() {
		defaultProfiles, err := loadBundleJSONProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load bundle json profiles: %s", err)
		}
		profiles = defaultProfiles
	} else {
		defaultProfiles, err := loadYamlProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load yaml profiles: %s", err)
		}
		profiles = defaultProfiles
	}
	for _, profileDef := range profiles {
		profiledefinition.NormalizeMetrics(profileDef.Definition.Metrics)
	}
	return profiles, nil
}

// GetProfileForSysObjectID return a profile for a sys object id
func GetProfileForSysObjectID(profiles ProfileConfigMap, sysObjectID string) (string, error) {
	tmpSysOidToProfile := map[string]string{}
	var matchedOids []string

	for profile, profConfig := range profiles {
		for _, oidPattern := range profConfig.Definition.SysObjectIds {
			found, err := filepath.Match(oidPattern, sysObjectID)
			if err != nil {
				log.Debugf("pattern error: %s", err)
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
					return "", fmt.Errorf("profile %s has the same sysObjectID (%s) as %s", profile, oidPattern, prevMatchedProfile)
				}
			}
			tmpSysOidToProfile[oidPattern] = profile
			matchedOids = append(matchedOids, oidPattern)
		}
	}
	oid, err := getMostSpecificOid(matchedOids)
	if err != nil {
		return "", fmt.Errorf("failed to get most specific profile for sysObjectID `%s`, for matched oids %v: %s", sysObjectID, matchedOids, err)
	}
	return tmpSysOidToProfile[oid], nil
}
