// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workflowjsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func Validate(schema *jsonschema.Schema, data any) error {
	return FormatValidationError(schema.Validate(data))
}

func FormatValidationError(err error) error {
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	ok := errors.As(err, &ve)
	if !ok {
		return err
	}
	if ve.KeywordLocation == "/required" {
		return errors.New(ve.Message)
	}
	// /conditions/comparator/0/foo -> .conditions.comparator.0.foo
	loc := strings.ReplaceAll(ve.InstanceLocation, "/", ".")
	if strings.HasSuffix(ve.KeywordLocation, "/anyOf") {
		return fmt.Errorf("%s: did not match any specified AnyOf schemas", loc)
	}
	if strings.HasSuffix(ve.KeywordLocation, "/additionalProperties") {
		return errors.New(ve.Message)
	}
	if len(ve.Causes) == 0 {
		return fmt.Errorf("%s: %s", loc, ve.Message)
	}
	var errs []error
	for _, c := range ve.Causes {
		if cErr := FormatValidationError(c); cErr != nil {
			errs = append(errs, cErr)
		}
	}
	return errors.Join(errs...)
}

// ValidateParameters validates parameters against the properties and required fields
// from an action parameter schema.
func ValidateParameters(parameterSchema map[string]interface{}, parameters any) error {
	schemaData := map[string]interface{}{
		"type":       "object",
		"properties": parameterSchema["properties"],
	}
	if required, found := parameterSchema["required"]; found {
		schemaData["required"] = required
	}

	schemaJSON, err := json.Marshal(schemaData)
	if err != nil {
		return fmt.Errorf("failed to marshal schema to JSON: %w", err)
	}

	schema, err := jsonschema.CompileString("parameter-schema.json", string(schemaJSON))
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	if err := Validate(schema, parameters); err != nil {
		return fmt.Errorf("parameter validation failed: %w", err)
	}
	return nil
}
