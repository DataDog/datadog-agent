// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

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
	reflector := jsonschema.Reflector{
		DoNotReference:            true,
		AllowAdditionalProperties: false,
		Mapper:                    jsonTypeMapper,
	}
	if ty == reflect.TypeOf(profiledefinition.JSONListMap[string]{}) {
		schema := reflector.Reflect([]profiledefinition.MapItem[string]{})
		schema.Version = ""
		return schema
	}
	if ty == reflect.TypeOf(profiledefinition.JSONListMap[profiledefinition.MetadataResourceConfig]{}) {
		schema := reflector.Reflect([]profiledefinition.MapItem[profiledefinition.MetadataResourceConfig]{})
		schema.Version = ""
		return schema
	}
	if ty == reflect.TypeOf(profiledefinition.JSONListMap[profiledefinition.MetadataField]{}) {
		schema := reflector.Reflect([]profiledefinition.MapItem[profiledefinition.MetadataField]{})
		schema.Version = ""
		return schema
	}
	return nil
}
