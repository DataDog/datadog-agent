// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcsnmpprofiles

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RemoteConfigSNMPProfilesManager receives configuration from remote-config
type RemoteConfigSNMPProfilesManager struct {
	upToDate bool
}

// NewRemoteConfigSNMPProfilesManager creates a new RemoteConfigSNMPProfilesManager.
func NewRemoteConfigSNMPProfilesManager() *RemoteConfigSNMPProfilesManager {
	return &RemoteConfigSNMPProfilesManager{
		upToDate: false,
	}
}

// Callback is when profiles updates are available (rc product NDM_DEVICE_PROFILES_CUSTOM)
func (rc *RemoteConfigSNMPProfilesManager) Callback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var profiles []profiledefinition.ProfileDefinition
	log.Info("RC Callback")
	for path, rawConfig := range updates {
		log.Infof("Path: %s", path)

		profileDef := profiledefinition.DeviceProfileRcConfig{}
		json.Unmarshal(rawConfig.Config, &profileDef)

		log.Infof("Profile Name: %s", profileDef.Profile.Name)
		log.Infof("Profile: %+v", profileDef.Profile)

		profiles = append(profiles, profileDef.Profile)
	}

	setGlobalRcProfiles(profiles)

	log.Infof("GetGlobalRcProfiles(): %+v", GetGlobalRcProfiles())
}
