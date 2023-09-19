// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/invopop/jsonschema"
	"reflect"
)

const MappingKeyPattern = "^\\d+$"

// GenerateJSONSchema generate jsonschema from profiledefinition.DeviceProfileRcConfig
func GenerateJSONSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		Mapper:                    jsonTypeMapper,
	}
	schema := reflector.Reflect(&profiledefinition.DeviceProfileRcConfig{})
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	schemaJSON = append(schemaJSON, byte('\n'))
	return schemaJSON, nil
}

func jsonTypeMapper(ty reflect.Type) *jsonschema.Schema {
	if ty == reflect.TypeOf(profiledefinition.ValueMapping{}) {
		return &jsonschema.Schema{
			Type: "object",
			PatternProperties: map[string]*jsonschema.Schema{
				MappingKeyPattern: {
					Type: "string",
				},
			},
		}
	} else if ty == reflect.TypeOf(profiledefinition.ValueMapping{}) {
		return &jsonschema.Schema{
			Type: "object",
			PatternProperties: map[string]*jsonschema.Schema{
				MappingKeyPattern: {
					Type: "string",
				},
			},
		}
	}
	return nil
}
