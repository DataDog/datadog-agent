// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/invopop/jsonschema"
)

const ValueMappingKeyPattern = "^\\d+$"
const TagMappingKeyPattern = "^[A-Za-z0-9-_]+$"
const MetadataResourceTypePattern = "^device|interface$"
const MetadataFieldTypePattern = "^[a-z]$"

// GenerateJSONSchema generate jsonschema from profiledefinition.DeviceProfileRcConfig
func GenerateJSONSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
	}
	schema := reflector.Reflect(&profiledefinition.DeviceProfileRcConfig{})
	normalizeSchema(schema)
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	schemaJSON = append(schemaJSON, byte('\n'))
	return schemaJSON, nil
}

func normalizeSchema(schema *jsonschema.Schema) {
	for key, def := range schema.Definitions {
		switch key {
		case "ValueMapping":
			def.PatternProperties = map[string]*jsonschema.Schema{
				ValueMappingKeyPattern: def.AdditionalProperties,
			}
			def.AdditionalProperties = nil
		case "TagsMapping":
			def.PatternProperties = map[string]*jsonschema.Schema{
				TagMappingKeyPattern: def.AdditionalProperties,
			}
			def.AdditionalProperties = nil
		case "MetadataConfig":
			def.PatternProperties = map[string]*jsonschema.Schema{
				MetadataResourceTypePattern: def.AdditionalProperties,
			}
			def.AdditionalProperties = nil
		case "FieldsConfig":
			def.PatternProperties = map[string]*jsonschema.Schema{
				MetadataFieldTypePattern: def.AdditionalProperties,
			}
			def.AdditionalProperties = nil
		}
	}
}
