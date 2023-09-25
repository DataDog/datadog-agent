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
											"key": "1",
											"value": "aa"
										},
										{
											"key": "2",
											"value": "bb"
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
											"key": "foo",
											"value": "$1"
										},
										{
											"key": "bar",
											"value": "$2"
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
