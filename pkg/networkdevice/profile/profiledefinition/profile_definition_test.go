package profiledefinition

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewProfileDefinition(t *testing.T) {
	tests := []struct {
		name            string
		inputJsonConfig string
		expectedProfile DeviceProfileRcConfig
	}{
		{
			name: "metric_tag mapping",
			inputJsonConfig: `
			{
				"profile_definition": {
					"name": "a-profile",
					"metrics": [
						{
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
									"mapping": [
										{
											"map_key": "1",
											"map_value": "aa"
										},
										{
											"map_key": "2",
											"map_value": "bb"
										}
									]
								}
							]
						}
					]
				}
			}`,
			expectedProfile: DeviceProfileRcConfig{
				Profile: ProfileDefinition{
					Name: "a-profile",
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
									Tag: "a-tag",
									Mapping: map[string]string{
										"1": "aa",
										"2": "bb",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "metric_tag match tags",
			inputJsonConfig: `
			{
				"profile_definition": {
					"name": "a-profile",
					"metrics": [
						{
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
									"match": "(\\d)(\\d)",
									"tags": [
										{
											"map_key": "foo",
											"map_value": "$1"
										},
										{
											"map_key": "bar",
											"map_value": "$2"
										}
									]
								}
							]
						}
					]
				}
			}`,
			expectedProfile: DeviceProfileRcConfig{
				Profile: ProfileDefinition{
					Name: "a-profile",
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
									Tag:   "a-tag",
									Match: "(\\d)(\\d)",
									Tags: map[string]string{
										"foo": "$1",
										"bar": "$2",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualProfile := DeviceProfileRcConfig{}
			err := json.Unmarshal([]byte(tt.inputJsonConfig), &actualProfile)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedProfile, actualProfile)
		})
	}
}
