// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

func mergeProfiles(profilesA ProfileConfigMap, profilesB ProfileConfigMap) ProfileConfigMap {
	profiles := make(ProfileConfigMap)
	for k, v := range profilesA {
		profiles[k] = v
	}
	for k, v := range profilesB {
		profiles[k] = v
	}
	//for k, v := range profilesA {
	//	profiles[k] = deepcopy.Copy(v).(ProfileConfig)
	//}
	//for k, v := range profilesB {
	//	profiles[k] = deepcopy.Copy(v).(ProfileConfig)
	//}
	return profiles
}
