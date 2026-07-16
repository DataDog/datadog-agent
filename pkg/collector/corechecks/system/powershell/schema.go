// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"encoding/json"
	"errors"
	"fmt"

	yy "github.com/ghodss/yaml"
	"github.com/swaggest/jsonschema-go"
	"github.com/xeipuuv/gojsonschema"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// createSchema reflects the instance-config JSON schema from instanceConfig.
//
// The top-level fields (required cmdlet, integer timeout, metrics minItems, ...)
// are derived from the struct tags — a single source of truth, exactly like the
// windows_certificate check. The four dual-form entry types each supply their
// own `oneOf` schema via JSONSchemaBytes (the jsonschema.RawExposer interface),
// so both the positional-tuple (JSON array) and mapping (JSON object) YAML shapes
// validate — a duality that exists in the YAML, not in the Go types, so it is the
// one part that must be stated explicitly.
func createSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{}
	schema, err := reflector.Reflect(instanceConfig{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(schema)
}

// JSONSchemaBytes lets metricEntry accept either a [property, name, type] tuple
// or a {property, name, type} mapping.
func (metricEntry) JSONSchemaBytes() ([]byte, error) {
	return []byte(`{
  "oneOf": [
    { "type": "array", "minItems": 2, "maxItems": 3 },
    {
      "type": "object",
      "required": ["property", "name"],
      "properties": {
        "property": {},
        "name": { "type": "string" },
        "type": {
          "type": "string",
          "enum": ["gauge", "rate", "count", "monotonic_count", "histogram", "distribution"]
        }
      }
    }
  ]
}`), nil
}

// JSONSchemaBytes lets filterEntry accept either a [Name, Value] tuple or a
// {name, value} mapping.
func (filterEntry) JSONSchemaBytes() ([]byte, error) {
	return []byte(`{
  "oneOf": [
    { "type": "array", "minItems": 2, "maxItems": 2 },
    { "type": "object" }
  ]
}`), nil
}

// JSONSchemaBytes lets tagByEntry accept either a "Property [AS alias]" string or
// a {property, alias} mapping.
func (tagByEntry) JSONSchemaBytes() ([]byte, error) {
	return []byte(`{
  "oneOf": [
    { "type": "string" },
    { "type": "object" }
  ]
}`), nil
}

// JSONSchemaBytes lets tagQueryEntry accept the 4-element positional tuple or a
// mapping.
func (tagQueryEntry) JSONSchemaBytes() ([]byte, error) {
	return []byte(`{
  "oneOf": [
    { "type": "array", "minItems": 4, "maxItems": 4 },
    { "type": "object" }
  ]
}`), nil
}

// validateInstanceSchema validates a single instance's raw YAML against the
// reflected schema. Every validation error is logged (so misconfigurations are
// visible at configure time), and a single aggregated error is returned when the
// instance is invalid.
func validateInstanceSchema(data []byte) error {
	schema, err := createSchema()
	if err != nil {
		return fmt.Errorf("could not build config schema: %w", err)
	}

	rawJSON, err := yy.YAMLToJSON(data)
	if err != nil {
		return fmt.Errorf("could not convert instance config to JSON for validation: %w", err)
	}

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schema),
		gojsonschema.NewBytesLoader(rawJSON),
	)
	if err != nil {
		return fmt.Errorf("could not run config schema validation: %w", err)
	}

	if !result.Valid() {
		for _, e := range result.Errors() {
			log.Errorf("powershell check: invalid config: %s", e)
		}
		return errors.New("powershell check config failed schema validation")
	}
	return nil
}
