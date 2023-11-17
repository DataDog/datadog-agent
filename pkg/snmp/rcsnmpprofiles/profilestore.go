// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcsnmpprofiles

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"sync"
)

var globalRcProfiles []profiledefinition.ProfileDefinition

var globalRcProfilesMu = &sync.Mutex{}

// setGlobalRcProfiles sets globalRcProfiles
func setGlobalRcProfiles(configMap []profiledefinition.ProfileDefinition) {
	globalRcProfilesMu.Lock()
	defer globalRcProfilesMu.Unlock()
	globalRcProfiles = configMap
}

// GetGlobalRcProfiles gets global globalProfileConfigMap
func GetGlobalRcProfiles() []profiledefinition.ProfileDefinition {
	globalRcProfilesMu.Lock()
	defer globalRcProfilesMu.Unlock()
	return globalRcProfiles
}
