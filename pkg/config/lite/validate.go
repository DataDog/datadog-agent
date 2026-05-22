// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

// Verdict is the outcome of ValidateRawConfig.
type Verdict int

const (
	// VerdictOK means the file parsed and passed schema validation
	VerdictOK Verdict = iota
	// VerdictYAMLParseFailure means yaml.Unmarshal returned an error
	VerdictYAMLParseFailure
	// VerdictSchemaInvalid means YAML parsed but the schema produced at least one error
	VerdictSchemaInvalid
	// VerdictSchemaUnavailable means the validator itself failed
	VerdictSchemaUnavailable
)

type ValidationResult struct {
	Verdict      Verdict
	ParseError   error
	SchemaErrors []string
	Parsed       map[string]any
}

// ValidateRawConfig uses the schema to validate a raw datadog.yaml
// Empty input is treated as VerdictOK
func ValidateRawConfig(raw []byte) ValidationResult {
	if len(raw) == 0 {
		return ValidationResult{Verdict: VerdictOK}
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return ValidationResult{Verdict: VerdictYAMLParseFailure, ParseError: err}
	}

	errs, schemaErr := schema.ValidateCoreConfig(parsed)
	switch {
	case schemaErr != nil:
		return ValidationResult{Verdict: VerdictSchemaUnavailable, Parsed: parsed}
	case len(errs) > 0:
		return ValidationResult{Verdict: VerdictSchemaInvalid, SchemaErrors: errs, Parsed: parsed}
	}
	return ValidationResult{Verdict: VerdictOK, Parsed: parsed}
}
