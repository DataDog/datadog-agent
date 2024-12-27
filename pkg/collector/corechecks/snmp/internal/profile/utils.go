// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"maps"
)

// mergeProfiles merges two or more ProfileConfigMaps. Later entries will
// supersede earlier ones.
func mergeProfiles(configMaps ...ProfileConfigMap) ProfileConfigMap {
	// Shallow-copy all entries from each overlay into a new map, then clone the
	// result. That way we avoid cloning profiles that get overlaid by later
	// versions.
	profiles := make(ProfileConfigMap)
	for _, overlay := range configMaps {
		maps.Copy(profiles, overlay)
	}
	return profiledefinition.CloneMap(profiles)
}
