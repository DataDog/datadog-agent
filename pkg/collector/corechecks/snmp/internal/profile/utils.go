// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import "github.com/mohae/deepcopy"

// mergeProfiles merges two profiles config map
// we use deepcopy to lower risk of modifying original profiles
func mergeProfiles(profilesA ProfileConfigMap, profilesB ProfileConfigMap) ProfileConfigMap {
	profiles := make(ProfileConfigMap)
	for k, v := range profilesA {
		profiles[k] = deepcopy.Copy(v).(ProfileConfig)
	}
	for k, v := range profilesB {
		profiles[k] = deepcopy.Copy(v).(ProfileConfig)
	}
	return profiles
}
