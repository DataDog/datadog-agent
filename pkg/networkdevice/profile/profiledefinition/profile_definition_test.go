// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDeviceProfileRcConfig_ConvertToRcFormat(t *testing.T) {
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
	newProfile := profile.convertToRcFormat()
	assert.Equal(t, expectedProfile, newProfile)
}

func TestDeviceProfileRcConfig_ConvertToAgentFormat(t *testing.T) {
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
	newProfile := profile.convertToAgentFormat()
	assert.Equal(t, expectedProfile, newProfile)
}

func TestDeviceProfileRcConfig_UnmarshallFromRc_and_MarshallForRc(t *testing.T) {
	// language=json
	rcConfig := []byte(`
{
	"profile_definition": {
		"name": "",
		"metrics": [
			{
				"table": {},
				"symbol": {},
				"symbols": [
					{
						"OID": "1.2.3",
						"name": "aSymbol"
					}
				],
				"metric_tags": [
					{
						"tag": "a-tag",
						"column": {
							"OID": "1.2.3",
							"name": "aSymbol"
						},
						"mapping_list": [
							{
								"key": "1",
								"value": "aa"
							},
							{
								"key": "2",
								"value": "bb"
							}
						]
					},
					{
						"tag": "a-tag2",
						"column": {
							"OID": "1.2.3",
							"name": "aSymbol"
						},
						"match": "(.*)(\\d+)",
						"tags_list": [
							{
								"key": "tag1",
								"value": "$1"
							},
							{
								"key": "tag2",
								"value": "$2"
							}
						]
					}
				]
			}
		],
		"metadata_list": [
			{
				"resource_type": "device",
				"fields_list": [
					{
						"symbol": {},
						"value": "my-device",
						"field_name": "name"
					}
				]
			}
		]
	}
}`)
	agentFormatProfile := &DeviceProfileRcConfig{
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
							Tag: "a-tag",
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
							Tag: "a-tag2",
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

	// Test Unmarshall
	newProfileAgentFormat, err := UnmarshallFromRc(rcConfig)
	assert.NoError(t, err)
	assert.Equal(t, agentFormatProfile, newProfileAgentFormat)

	// Test Marshall
	//newProfileAgentFormatBytes, err := agentFormatProfile.MarshallForRc()
	//assert.NoError(t, err)
	//assert.JSONEq(t, string(rcConfig), string(newProfileAgentFormatBytes))
	//
	//// Test Unmarshall + Marshall
	//newProfileAgentFormat2, err := UnmarshallFromRc(rcConfig)
	//assert.NoError(t, err)
	//newProfileAgentFormatBytes2, err := newProfileAgentFormat2.MarshallForRc()
	//assert.NoError(t, err)
	//assert.JSONEq(t, string(rcConfig), string(newProfileAgentFormatBytes2))
}
