package profiledefinition

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewProfileDefinition(t *testing.T) {
	// language=json
	rcConfig := []byte(`
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
}`)

	profile := DeviceProfileRcConfig{}

	err := json.Unmarshal(rcConfig, &profile)
	assert.NoError(t, err)

	expectedProfile := DeviceProfileRcConfig{
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
	}
	assert.Equal(t, expectedProfile, profile)
}
