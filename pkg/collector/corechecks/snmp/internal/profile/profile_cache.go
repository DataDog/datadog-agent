// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profile

var globalProfileConfigMap ProfileConfigMap

// SetGlobalProfileConfigMap sets global globalProfileConfigMap
func SetGlobalProfileConfigMap(configMap ProfileConfigMap) {
	globalProfileConfigMap = configMap
}

// GetGlobalProfileConfigMap gets global globalProfileConfigMap
func GetGlobalProfileConfigMap() ProfileConfigMap {
	return globalProfileConfigMap
}
