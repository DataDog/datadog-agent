// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// SetConfdPathAndCleanProfiles is used for testing only
func SetConfdPathAndCleanProfiles() {
	globalProfileConfigMap = nil // make sure from the new confd path will be reloaded
	file, _ := filepath.Abs(filepath.Join(".", "test", "conf.d"))
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join("..", "test", "conf.d"))
	}
	if !pathExists(file) {
		file, _ = filepath.Abs(filepath.Join(".", "internal", "test", "conf.d"))
	}
	config.Datadog.Set("confd_path", file)
}

// pathExists returns true if the given path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// copyProfileDefinition copies a profile, it's used for testing
func copyProfileDefinition(profileDef profiledefinition.ProfileDefinition) profiledefinition.ProfileDefinition {
	newDef := profiledefinition.ProfileDefinition{}
	newDef.Metrics = append(newDef.Metrics, profileDef.Metrics...)
	newDef.MetricTags = append(newDef.MetricTags, profileDef.MetricTags...)
	newDef.StaticTags = append(newDef.StaticTags, profileDef.StaticTags...)
	newDef.Metadata = make(profiledefinition.MetadataConfig)
	newDef.Device = profileDef.Device
	newDef.Extends = append(newDef.Extends, profileDef.Extends...)
	newDef.SysObjectIds = append(newDef.SysObjectIds, profileDef.SysObjectIds...)

	for resName, resource := range profileDef.Metadata {
		resConfig := profiledefinition.MetadataResourceConfig{}
		resConfig.Fields = make(map[string]profiledefinition.MetadataField)
		for fieldName, field := range resource.Fields {
			resConfig.Fields[fieldName] = field
		}
		resConfig.IDTags = append(resConfig.IDTags, resource.IDTags...)
		newDef.Metadata[resName] = resConfig
	}
	return newDef
}
