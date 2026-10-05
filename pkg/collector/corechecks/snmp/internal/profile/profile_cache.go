// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

var globalProfileConfigMap ProfileConfigMap
var globalLegacyProfiles []string

// SetGlobalProfileConfigMap sets global globalProfileConfigMap
func SetGlobalProfileConfigMap(configMap ProfileConfigMap) {
	setGlobalProfiles(configMap, nil)
}

// setGlobalProfiles caches the profiles together with the names of the profiles
// that use the legacy Python syntax. The two must be cached together: callers of
// loadYamlProfiles need the legacy profile names on cache hits as well, otherwise
// the loader picked for a check instance depends on whether that instance happened
// to be the one that populated the cache.
func setGlobalProfiles(configMap ProfileConfigMap, legacyProfiles []string) {
	globalProfileConfigMap = configMap
	globalLegacyProfiles = legacyProfiles
}

// GetGlobalProfileConfigMap gets global globalProfileConfigMap
func GetGlobalProfileConfigMap() ProfileConfigMap {
	return globalProfileConfigMap
}

// getGlobalLegacyProfiles gets the names of the cached profiles using the legacy Python syntax
func getGlobalLegacyProfiles() []string {
	return globalLegacyProfiles
}
