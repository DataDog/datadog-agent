// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"

	workflowjsonschema "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/workflowjsonschema"
)

func validateParameters(params interface{}, parameterSchema map[string]interface{}) error {
	schemaData := map[string]interface{}{
		"type":       "object",
		"properties": parameterSchema["properties"],
	}
	if req, ok := parameterSchema["required"]; ok {
		schemaData["required"] = req
	}

	schemaJSON, err := json.Marshal(schemaData)
	if err != nil {
		return fmt.Errorf("failed to marshal schema to JSON: %w", err)
	}

	schema, err := jsonschema.CompileString("parameter-schema.json", string(schemaJSON))
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	if err := workflowjsonschema.Validate(schema, params); err != nil {
		return fmt.Errorf("parameter validation failed: %w", err)
	}
	return nil
}
