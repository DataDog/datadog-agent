// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// ProfileConfigMap represent a map of ProfileConfig
type ProfileConfigMap map[string]ProfileConfig

// ProfileConfig represent a profile configuration
type ProfileConfig struct {
	DefinitionFile string                              `yaml:"definition_file"`
	Definition     profiledefinition.ProfileDefinition `yaml:"definition"`

	IsUserProfile bool `yaml:"-"`
}

// CopyProfileDefinition copies a profile, it's used for testing
// TODO: Use deepcopy library instead?
func CopyProfileDefinition(profileDef profiledefinition.ProfileDefinition) profiledefinition.ProfileDefinition {
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
