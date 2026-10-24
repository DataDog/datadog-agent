// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workflowjsonschema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var messagePrinter = message.NewPrinter(language.English)

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
	if _, ok := ve.ErrorKind.(*kind.Required); ok && len(ve.InstanceLocation) == 0 {
		return errors.New(ve.ErrorKind.LocalizedString(messagePrinter))
	}
	// [conditions comparator 0 foo] -> .conditions.comparator.0.foo
	loc := strings.Join(append([]string{""}, ve.InstanceLocation...), ".")
	if _, ok := ve.ErrorKind.(*kind.AnyOf); ok {
		return fmt.Errorf("%s: did not match any specified AnyOf schemas", loc)
	}
	if _, ok := ve.ErrorKind.(*kind.AdditionalProperties); ok {
		return errors.New(ve.ErrorKind.LocalizedString(messagePrinter))
	}
	if len(ve.Causes) == 0 {
		return fmt.Errorf("%s: %s", loc, ve.ErrorKind.LocalizedString(messagePrinter))
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

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	const loc = "parameter-schema.json"
	c := jsonschema.NewCompiler()
	if err := c.AddResource(loc, doc); err != nil {
		return fmt.Errorf("failed to add schema resource: %w", err)
	}

	schema, err := c.Compile(loc)
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	if err := Validate(schema, parameters); err != nil {
		return fmt.Errorf("parameter validation failed: %w", err)
	}
	return nil
}
