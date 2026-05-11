// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

// Verdict is the outcome of running ValidateRawConfig against datadog.yaml.
type Verdict int

const (
	// VerdictOK means the file parsed as YAML and the parsed map passed
	// schema validation.
	VerdictOK Verdict = iota
	// VerdictYAMLParseFailure means yaml.Unmarshal returned an error. The
	// configuration cannot be loaded at all.
	VerdictYAMLParseFailure
	// VerdictSchemaInvalid means yaml.Unmarshal succeeded but
	// schema.ValidateCoreConfig produced at least one error string.
	VerdictSchemaInvalid
	// VerdictSchemaUnavailable means the validator itself failed (e.g.
	// the embedded schema file is missing). The caller should log a
	// warning and treat this as "no opinion".
	VerdictSchemaUnavailable
)

// ValidationResult is what ValidateRawConfig returns. Callers inspect Verdict
// first and only use the matching fields.
type ValidationResult struct {
	Verdict      Verdict
	ParseError   error
	SchemaErrors []string
	Parsed       map[string]any
}

// ValidateRawConfig is the single source of truth for "parse datadog.yaml,
// run it through the embedded schema, summarise the result." It is used by
// both the in-Fx invalidconfig issue module and lite.Rescue so the two
// paths emit consistent issue payloads regardless of who detected the
// problem.
//
// Empty input is treated as VerdictOK — there is no file to complain about.
func ValidateRawConfig(raw []byte) ValidationResult {
	if len(raw) == 0 {
		return ValidationResult{Verdict: VerdictOK}
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return ValidationResult{
			Verdict:    VerdictYAMLParseFailure,
			ParseError: err,
		}
	}

	errs, schemaErr := schema.ValidateCoreConfig(parsed)
	if schemaErr != nil {
		return ValidationResult{
			Verdict: VerdictSchemaUnavailable,
			Parsed:  parsed,
		}
	}
	if len(errs) > 0 {
		return ValidationResult{
			Verdict:      VerdictSchemaInvalid,
			SchemaErrors: errs,
			Parsed:       parsed,
		}
	}
	return ValidationResult{Verdict: VerdictOK, Parsed: parsed}
}
