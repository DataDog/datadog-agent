// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build jsonschema || test

package profiledefinition

import "github.com/invopop/jsonschema"

// JSONSchema defines the JSON schema for MetadataConfig
func (mc MetadataConfig) JSONSchema() *jsonschema.Schema {
	return ListMap[MetadataResourceConfig](mc).JSONSchema()
}

// JSONSchema is needed to customize jsonschema to match []MapItem[T] used in json format
func (lm ListMap[T]) JSONSchema() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect([]MapItem[T]{})
	// don't need version because this is a child of a versioned schema.
	schema.Version = ""
	return schema
}
