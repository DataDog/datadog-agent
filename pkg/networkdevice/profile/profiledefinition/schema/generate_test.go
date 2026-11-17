// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGenerateJSONSchemaIsInSync(t *testing.T) {
	schemaJSON, err := GenerateJSONSchema()
	require.NoError(t, err)

	assert.JSONEq(t, string(GetDeviceProfileRcConfigJsonschema()), string(schemaJSON))
}

type ExampleStruct struct {
	Mapping profiledefinition.ListMap[string] `json:"mapping"`
}

func TestListMap_JSONSchema(t *testing.T) {
	schema := Reflect(&ExampleStruct{})
	schema.Version = "" // no version, to make the test more future-proof
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	expectedSchema := `
{
  "$id": "https://github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition/schema/example-struct",
  "$ref": "#/$defs/ExampleStruct",
  "$defs": {
    "ExampleStruct": {
      "properties": {
        "mapping": {
          "$ref": "#/$defs/ListMap[string]"
        }
      },
      "additionalProperties": false,
      "type": "object",
      "required": [
        "mapping"
      ]
    },
    "ListMap[string]": {
      "items": {
        "properties": {
          "key": {
            "type": "string"
          },
          "value": {
            "type": "string"
          }
        },
        "additionalProperties": false,
        "type": "object",
        "required": [
          "key",
          "value"
        ]
      },
      "type": "array"
    }
  }
}
`
	assert.JSONEq(t, expectedSchema, string(schemaJSON))
}
