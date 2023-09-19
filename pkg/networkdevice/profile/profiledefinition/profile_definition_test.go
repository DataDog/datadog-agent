// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDeviceProfileRcConfig_NormalizeForRc(t *testing.T) {
	profile := &DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			Metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "my-device",
						},
					},
				},
			},
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Mapping: map[string]string{
								"1": "aa",
								"2": "bb",
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							Tags: map[string]string{
								"tag1": "$1",
								"tag2": "$2",
							},
						},
					},
				},
			},
		},
	}
	expectedProfile := &DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			MetadataList: []MetadataResourceConfig{
				{
					ResourceType: "device",
					FieldsList: []MetadataField{
						{
							Value:     "my-device",
							FieldName: "name",
						},
					},
				},
			},
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							MappingList: []KeyValue{
								{
									Key:   "1",
									Value: "aa",
								},
								{
									Key:   "2",
									Value: "bb",
								},
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							TagsList: []KeyValue{
								{
									Key:   "tag1",
									Value: "$1",
								},
								{
									Key:   "tag2",
									Value: "$2",
								},
							},
						},
					},
				},
			},
		},
	}
	newProfile := profile.NormalizeForRc()
	assert.Equal(t, expectedProfile, newProfile)
}

func TestDeviceProfileRcConfig_NormalizeForAgent(t *testing.T) {
	profile := &DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			MetadataList: []MetadataResourceConfig{
				{
					ResourceType: "device",
					FieldsList: []MetadataField{
						{
							Value:     "my-device",
							FieldName: "name",
						},
					},
				},
			},
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							MappingList: []KeyValue{
								{
									Key:   "1",
									Value: "aa",
								},
								{
									Key:   "2",
									Value: "bb",
								},
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							TagsList: []KeyValue{
								{
									Key:   "tag1",
									Value: "$1",
								},
								{
									Key:   "tag2",
									Value: "$2",
								},
							},
						},
					},
				},
			},
		},
	}
	expectedProfile := &DeviceProfileRcConfig{
		Profile: ProfileDefinition{
			Metadata: MetadataConfig{
				"device": MetadataResourceConfig{
					Fields: map[string]MetadataField{
						"name": {
							Value: "my-device",
						},
					},
				},
			},
			Metrics: []MetricsConfig{
				{
					Symbols: []SymbolConfig{
						{
							OID:  "1.2.3",
							Name: "aSymbol",
						},
					},
					MetricTags: MetricTagConfigList{
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Mapping: map[string]string{
								"1": "aa",
								"2": "bb",
							},
						},
						{
							Column: SymbolConfig{
								OID:  "1.2.3",
								Name: "aSymbol",
							},
							Match: "(.*)(\\d+)",
							Tags: map[string]string{
								"tag1": "$1",
								"tag2": "$2",
							},
						},
					},
				},
			},
		},
	}
	newProfile := profile.NormalizeForAgent()
	assert.Equal(t, expectedProfile, newProfile)
}
