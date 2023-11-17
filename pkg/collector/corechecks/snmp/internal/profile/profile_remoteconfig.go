// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/snmp/rcsnmpprofiles"
	"sync"
)

var rcProfilesMu = &sync.Mutex{}

func loadRemoteConfigProfiles() (ProfileConfigMap, error) {
	rcProfilesMu.Lock()
	defer rcProfilesMu.Unlock()

	profiles := make(ProfileConfigMap)

	rcProfiles := rcsnmpprofiles.GetGlobalRcProfiles()
	for _, profile := range rcProfiles {
		// TODO: check invalid/empty name
		// TODO: check duplicate name
		// TODO: merge logic with unmarshallProfilesBundleJSON ?
		//       https://github.com/DataDog/datadog-agent/blob/a46463055734524ce10314d069a567f28f9033ba/pkg/collector/corechecks/snmp/internal/profile/profile_json_bundle.go#L46
		profiles[profile.Name] = ProfileConfig{
			Definition:    profile,
			IsUserProfile: true,
		}
	}
	return profiles, nil
}
