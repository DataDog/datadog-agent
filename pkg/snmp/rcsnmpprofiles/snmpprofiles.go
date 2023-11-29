// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcsnmpprofiles

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	parse "github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
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
	log.Info("[RC Callback] RC Callback")
	for path, rawConfig := range updates {
		log.Infof("[RC Callback] Path: %s", path)

		profileDef := profiledefinition.DeviceProfileRcConfig{}
		json.Unmarshal(rawConfig.Config, &profileDef)

		log.Infof("[RC Callback] Profile Name: %s", profileDef.Profile.Name)
		log.Infof("[RC Callback] Profile: %+v", profileDef.Profile)

		profiles = append(profiles, profileDef.Profile)
	}

	// TODO: Do not collect snmp-listener configs
	snmpConfigList, err := parse.GetConfigCheckSnmp()
	if err != nil {
		log.Infof("[RC Callback] Couldn't parse the SNMP config: %v", err)
		return
	}
	log.Infof("[RC Callback] snmpConfigList len=%d", len(snmpConfigList))

	for _, config := range snmpConfigList {
		log.Infof("[RC Callback] SNMP config: %+v", config)
	}
}
