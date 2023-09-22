// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mohae/deepcopy"
)

// DeviceMeta holds device related static metadata
// DEPRECATED in favour of profile metadata syntax
type DeviceMeta struct {
	// deprecated in favour of new `ProfileDefinition.Metadata` syntax
	Vendor string `yaml:"vendor,omitempty" json:"vendor,omitempty"`
}

// ProfileDefinition is the root profile structure
type ProfileDefinition struct {
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	SysObjectIds StringArray       `yaml:"sysobjectid,omitempty" json:"sysobjectid,omitempty"`
	Extends      []string          `yaml:"extends,omitempty" json:"extends,omitempty"`
	Metadata     MetadataConfig    `yaml:"metadata,omitempty" json:"-" jsonschema:"-"` // not exposed as json annotation since MetadataList is used instead
	MetricTags   []MetricTagConfig `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
	StaticTags   []string          `yaml:"static_tags,omitempty" json:"static_tags,omitempty"`
	Metrics      []MetricsConfig   `yaml:"metrics,omitempty" json:"metrics,omitempty"`

	Device DeviceMeta `yaml:"device,omitempty" json:"-" jsonschema:"-"` // DEPRECATED

	// Used in RC format (list instead of map)
	MetadataList []MetadataResourceConfig `yaml:"-" json:"-"`
}

// DeviceProfileRcConfig represent the profile stored in remote config.
type DeviceProfileRcConfig struct {
	Profile ProfileDefinition `json:"profile_definition"`
}

// NewProfileDefinition creates a new ProfileDefinition
func NewProfileDefinition() *ProfileDefinition {
	p := &ProfileDefinition{}
	p.Metadata = make(MetadataConfig)
	return p
}

type DeviceProfileRcConfigCustom DeviceProfileRcConfig

func (d *DeviceProfileRcConfigCustom) UnmarshalJSON(data []byte) error {
	profile := DeviceProfileRcConfig{}
	err := json.Unmarshal(data, &profile)
	if err != nil {
		return err
	}

	var deviceProfileRcConfig map[string]*json.RawMessage
	err = json.Unmarshal(data, &deviceProfileRcConfig)
	if err != nil {
		return err
	}

	profileDef := deviceProfileRcConfig["profile_definition"]
	fmt.Println(deviceProfileRcConfig)
	fmt.Println(profileDef)
	if deviceProfileRcConfig["profile_definition"] != nil {
		var profileDefinition map[string]*json.RawMessage
		json.Unmarshal(*deviceProfileRcConfig["profile_definition"], &profileDefinition)
		fmt.Println(profileDefinition)
		if profileDefinition["metadata_list"] != nil {
			var metadataResourceConfigList []*json.RawMessage
			json.Unmarshal(*profileDefinition["metadata_list"], &metadataResourceConfigList)

			profile.Profile.Metadata = make(MetadataConfig)
			for _, metadataResourceConfigRaw := range metadataResourceConfigList {
				var metadataResourceConfigRawMap map[string]*json.RawMessage
				json.Unmarshal(*metadataResourceConfigRaw, &metadataResourceConfigRawMap)
				var resourceType string
				json.Unmarshal(*metadataResourceConfigRawMap["resource_type"], &resourceType)
				var metadataResourceConfig MetadataResourceConfig
				json.Unmarshal(*metadataResourceConfigRaw, &metadataResourceConfig)
				profile.Profile.Metadata[resourceType] = metadataResourceConfig
			}
			fmt.Println(metadataResourceConfigList)

		}
	}
	//profile.Profile.MetadataList = nil
	*d = DeviceProfileRcConfigCustom(profile)
	return nil
}

// UnmarshallFromRc creates a new ProfileDefinition from RC Config []byte
func UnmarshallFromRc(config []byte) (*DeviceProfileRcConfig, error) {
	profile := &DeviceProfileRcConfigCustom{}
	err := json.NewDecoder(bytes.NewReader(config)).Decode(&profile)
	//err := json.Unmarshal(config, profile)
	if err != nil {
		return nil, err
	}
	newProfile := DeviceProfileRcConfig(*profile)
	return &newProfile, nil
}

// MarshallForRc to []byte for RC.
func (d *DeviceProfileRcConfig) MarshallForRc() ([]byte, error) {
	conf, err := json.Marshal(d.convertToRcFormat())
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// deepCopy make a deepcopy
func (d *DeviceProfileRcConfig) deepCopy() *DeviceProfileRcConfig {
	newProfile := deepcopy.Copy(*d).(DeviceProfileRcConfig)
	return &newProfile
}

// convertToRcFormat will normalize the device profile in-place to make it suitable for RC
// This operation is opposite to convertToAgentFormat
// Profiles are converted into RC format before being stored in RC.
func (d *DeviceProfileRcConfig) convertToRcFormat() *DeviceProfileRcConfig {
	newProfile := d.deepCopy()
	for i := range newProfile.Profile.Metrics {
		metric := &newProfile.Profile.Metrics[i]
		for j := range metric.MetricTags {
			metricTag := &metric.MetricTags[j]
			// Convert Mapping
			if len(metricTag.Mapping) > 0 {
				metricTag.MappingList = []KeyValue{}
				var keys []string
				for key := range metricTag.Mapping {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					val := metricTag.Mapping[key]
					metricTag.MappingList = append(metricTag.MappingList, KeyValue{
						Key:   key,
						Value: val,
					})
				}
				metricTag.Mapping = nil
			}

			// Convert Tags
			if len(metricTag.Tags) > 0 {
				metricTag.TagsList = []KeyValue{}
				var keys []string
				for key := range metricTag.Tags {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					val := metricTag.Tags[key]
					metricTag.TagsList = append(metricTag.TagsList, KeyValue{
						Key:   key,
						Value: val,
					})
				}
				metricTag.Tags = nil
			}
		}
	}

	if len(newProfile.Profile.Metadata) > 0 {
		newProfile.Profile.MetadataList = []MetadataResourceConfig{}
		var metaResourceKeys []string
		for metaResource := range newProfile.Profile.Metadata {
			metaResourceKeys = append(metaResourceKeys, metaResource)
		}
		sort.Strings(metaResourceKeys)
		for _, key := range metaResourceKeys {
			metadataConfig := newProfile.Profile.Metadata[key]
			metadataConfig.ResourceType = key
			newProfile.Profile.MetadataList = append(newProfile.Profile.MetadataList, metadataConfig)
		}
		newProfile.Profile.Metadata = nil
	}

	for i := range newProfile.Profile.MetadataList {
		metadata := &newProfile.Profile.MetadataList[i]
		if len(metadata.Fields) > 0 {
			metadata.FieldsList = []MetadataField{}
			var fieldNames []string
			for fieldName := range metadata.Fields {
				fieldNames = append(fieldNames, fieldName)
			}
			sort.Strings(fieldNames)
			for _, key := range fieldNames {
				fieldConfig := metadata.Fields[key]
				fieldConfig.FieldName = key
				metadata.FieldsList = append(metadata.FieldsList, fieldConfig)
			}
			metadata.Fields = nil
		}
	}
	return newProfile
}

// convertToAgentFormat will normalize the device profile in-place to make it suitable for Agent
// This operation is opposite to convertToRcFormat.
// After retrieved a Profile from RC, it should be converted into Agent Format to be used elsewhere.
func (d *DeviceProfileRcConfig) convertToAgentFormat() *DeviceProfileRcConfig {
	newProfile := d.deepCopy()
	for i := range newProfile.Profile.Metrics {
		metric := &newProfile.Profile.Metrics[i]
		for j := range metric.MetricTags {
			metricTag := &metric.MetricTags[j]
			// Convert Mapping
			if len(metricTag.MappingList) > 0 {
				metricTag.Mapping = map[string]string{}
				for _, entry := range metricTag.MappingList {
					metricTag.Mapping[entry.Key] = entry.Value
				}
				metricTag.MappingList = nil
			}

			// Convert Tags
			if len(metricTag.TagsList) > 0 {
				metricTag.Tags = map[string]string{}
				for _, entry := range metricTag.TagsList {
					metricTag.Tags[entry.Key] = entry.Value
				}
				metricTag.TagsList = nil
			}
		}
	}
	if len(newProfile.Profile.MetadataList) > 0 {
		newProfile.Profile.Metadata = make(MetadataConfig)
		for _, item := range newProfile.Profile.MetadataList {
			resourceType := item.ResourceType
			item.ResourceType = ""
			newProfile.Profile.Metadata[resourceType] = item
		}
		newProfile.Profile.MetadataList = nil
	}
	for key := range newProfile.Profile.Metadata {
		metadata := newProfile.Profile.Metadata[key]
		if len(metadata.FieldsList) > 0 {
			metadata.Fields = make(map[string]MetadataField)
			for _, field := range metadata.FieldsList {
				fieldName := field.FieldName
				field.FieldName = ""
				metadata.Fields[fieldName] = field
			}
			metadata.FieldsList = nil
		}
		newProfile.Profile.Metadata[key] = metadata
	}
	return newProfile
}
