// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import "sort"

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
	Metadata     MetadataConfig    `yaml:"metadata,omitempty" json:"metadata,omitempty" jsonschema:"-"`
	MetricTags   []MetricTagConfig `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
	StaticTags   []string          `yaml:"static_tags,omitempty" json:"static_tags,omitempty"`
	Metrics      []MetricsConfig   `yaml:"metrics,omitempty" json:"metrics,omitempty"`

	Device DeviceMeta `yaml:"device,omitempty" json:"device,omitempty" jsonschema:"-"` // DEPRECATED

	// Used to convert into RC format (list instead of map)
	MetadataList []MetadataResourceConfig `yaml:"-" json:"metadata_list,omitempty"`
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

func (d *DeviceProfileRcConfig) NormalizeInplaceForRc() {
	for i := range d.Profile.Metrics {
		metric := &d.Profile.Metrics[i]
		for j := range metric.MetricTags {
			metricTag := &metric.MetricTags[j]
			// Normalize Mapping
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

			// Normalize Tags
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

	if len(d.Profile.Metadata) > 0 {
		d.Profile.MetadataList = []MetadataResourceConfig{}
		var metaResourceKeys []string
		for metaResource := range d.Profile.Metadata {
			metaResourceKeys = append(metaResourceKeys, metaResource)
		}
		sort.Strings(metaResourceKeys)
		for _, key := range metaResourceKeys {
			metadataConfig := d.Profile.Metadata[key]
			metadataConfig.ResourceType = key
			d.Profile.MetadataList = append(d.Profile.MetadataList, metadataConfig)
		}
		d.Profile.Metadata = nil
	}

	for i := range d.Profile.MetadataList {
		metadata := &d.Profile.MetadataList[i]
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
}

func (d *DeviceProfileRcConfig) NormalizeInplaceFromRc() {
	for i := range d.Profile.Metrics {
		metric := &d.Profile.Metrics[i]
		for j := range metric.MetricTags {
			metricTag := &metric.MetricTags[j]
			// Normalize Mapping
			if len(metricTag.MappingList) > 0 {
				metricTag.Mapping = map[string]string{}
				for _, entry := range metricTag.MappingList {
					metricTag.Mapping[entry.Key] = entry.Value
				}
				metricTag.MappingList = nil
			}

			// Normalize Tags
			if len(metricTag.TagsList) > 0 {
				metricTag.Tags = map[string]string{}
				for _, entry := range metricTag.TagsList {
					metricTag.Tags[entry.Key] = entry.Value
				}
				metricTag.TagsList = nil
			}
		}
	}
	if len(d.Profile.MetadataList) > 0 {
		d.Profile.Metadata = make(MetadataConfig)
		for _, item := range d.Profile.MetadataList {
			resourceType := item.ResourceType
			item.ResourceType = ""
			d.Profile.Metadata[resourceType] = item
		}
		d.Profile.MetadataList = nil
	}
	for key := range d.Profile.Metadata {
		metadata := d.Profile.Metadata[key]
		if len(metadata.FieldsList) > 0 {
			metadata.Fields = make(map[string]MetadataField)
			for _, field := range metadata.FieldsList {
				fieldName := field.FieldName
				field.FieldName = ""
				metadata.Fields[fieldName] = field
			}
			metadata.FieldsList = nil
		}
		d.Profile.Metadata[key] = metadata
	}
}
