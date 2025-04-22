// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"os"
)

// mergeProfiles merges two ProfileConfigMaps
// we clone the profiles to lower the risk of modifying original profiles
func mergeProfiles(profilesA ProfileConfigMap, profilesB ProfileConfigMap) ProfileConfigMap {
	profiles := make(ProfileConfigMap)
	for k, v := range profilesA {
		profiles[k] = v.Clone()
	}
	for k, v := range profilesB {
		profiles[k] = v.Clone()
	}
	return profiles
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
