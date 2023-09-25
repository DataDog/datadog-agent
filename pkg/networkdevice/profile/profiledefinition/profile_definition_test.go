package profiledefinition

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewProfileDefinition(t *testing.T) {

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
	fmt.Printf("%+v", profile)
}
