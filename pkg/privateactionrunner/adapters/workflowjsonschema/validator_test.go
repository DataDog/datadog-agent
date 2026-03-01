// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package workflowjsonschema

import (
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatValidationError_NilReturnsNil verifies the trivial case: a nil input
// produces a nil output (no error wrapping).
func TestFormatValidationError_NilReturnsNil(t *testing.T) {
	assert.NoError(t, FormatValidationError(nil))
}

// TestFormatValidationError_NonJsonschemaErrorPassthrough verifies that a plain error
// (not a *jsonschema.ValidationError) is returned unchanged, so callers can distinguish
// schema-validation failures from other errors.
func TestFormatValidationError_NonJsonschemaErrorPassthrough(t *testing.T) {
	sentinel := errors.New("some other error")
	result := FormatValidationError(sentinel)
	assert.Equal(t, sentinel, result)
}

// TestFormatValidationError_RequiredKeyword verifies that a top-level "/required"
// keyword location returns a concise error containing only the message (no path prefix),
// since "required" errors already have a human-readable message from the library.
func TestFormatValidationError_RequiredKeyword(t *testing.T) {
	ve := &jsonschema.ValidationError{
		KeywordLocation:  "/required",
		InstanceLocation: "",
		Message:          "missing properties: 'name', 'age'",
	}

	err := FormatValidationError(ve)

	require.Error(t, err)
	assert.Equal(t, "missing properties: 'name', 'age'", err.Error())
}

// TestFormatValidationError_AnyOfKeyword verifies that an "/anyOf" suffix in the
// keyword location produces an informative "did not match any specified AnyOf schemas"
// message with the instance path prepended.
func TestFormatValidationError_AnyOfKeyword(t *testing.T) {
	ve := &jsonschema.ValidationError{
		KeywordLocation:  "/properties/action/anyOf",
		InstanceLocation: "/action",
		Message:          "oneOf conditions not met",
	}

	err := FormatValidationError(ve)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not match any specified AnyOf schemas")
	// Instance location "/" → "." via replacement; "/action" → ".action"
	assert.Contains(t, err.Error(), ".action")
}

// TestFormatValidationError_AdditionalPropertiesKeyword verifies that an
// "/additionalProperties" keyword location returns the raw message, which already
// names the unexpected property key.
func TestFormatValidationError_AdditionalPropertiesKeyword(t *testing.T) {
	ve := &jsonschema.ValidationError{
		KeywordLocation:  "/properties/config/additionalProperties",
		InstanceLocation: "/config",
		Message:          "additionalProperties 'secret' not allowed",
	}

	err := FormatValidationError(ve)

	require.Error(t, err)
	assert.Equal(t, "additionalProperties 'secret' not allowed", err.Error())
}

// TestFormatValidationError_LeafErrorWithLocation verifies that a validation error with
// no causes formats as "<instance.path>: <message>" so the caller can point the user to
// the exact location of the invalid value.
func TestFormatValidationError_LeafErrorWithLocation(t *testing.T) {
	ve := &jsonschema.ValidationError{
		KeywordLocation:  "/properties/timeout/type",
		InstanceLocation: "/timeout",
		Message:          "expected number, but got string",
	}

	err := FormatValidationError(ve)

	require.Error(t, err)
	// "/timeout" → ".timeout" after replacement
	assert.Contains(t, err.Error(), ".timeout")
	assert.Contains(t, err.Error(), "expected number, but got string")
}

// TestFormatValidationError_NestedCausesCollected verifies that when a validation error
// has Causes, the function recurses into them and returns a multierror containing each
// leaf error — not the parent message, which is often empty or redundant.
func TestFormatValidationError_NestedCausesCollected(t *testing.T) {
	child1 := &jsonschema.ValidationError{
		KeywordLocation:  "/properties/name/type",
		InstanceLocation: "/name",
		Message:          "expected string, but got number",
	}
	child2 := &jsonschema.ValidationError{
		KeywordLocation:  "/properties/count/type",
		InstanceLocation: "/count",
		Message:          "expected integer, but got string",
	}
	parent := &jsonschema.ValidationError{
		KeywordLocation:  "/properties",
		InstanceLocation: "",
		Message:          "",
		Causes:           []*jsonschema.ValidationError{child1, child2},
	}

	err := FormatValidationError(parent)

	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "expected string, but got number")
	assert.Contains(t, msg, "expected integer, but got string")
}

// TestFormatValidationError_RootInstanceLocation verifies that the root instance
// location ("/") is converted to "." correctly (no leading slash noise).
func TestFormatValidationError_RootInstanceLocation(t *testing.T) {
	ve := &jsonschema.ValidationError{
		KeywordLocation:  "/type",
		InstanceLocation: "/",
		Message:          "expected object, but got array",
	}

	err := FormatValidationError(ve)

	require.Error(t, err)
	// "/" → "." after ReplaceAll
	assert.Contains(t, err.Error(), ".")
	assert.Contains(t, err.Error(), "expected object, but got array")
}
