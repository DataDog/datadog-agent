// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schemaimpl

import (
	"errors"
	"math/big"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/assert/yaml"
	"github.com/stretchr/testify/require"
)

// secretValue stands for a configured value: no reason may contain it
const secretValue = "PRIVATE_VALUE"

func TestViolationReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind jsonschema.ErrorKind
		want string
	}{
		{"type", &kind.Type{Got: "number", Want: []string{"string"}}, "got number instead of string"},
		{"type with two types", &kind.Type{Got: "number", Want: []string{"string", "array"}}, "got number instead of string or array"},
		{"type with many types", &kind.Type{Got: "number", Want: []string{"string", "array", "object"}}, "got number instead of string, array or object"},
		{"enum", &kind.Enum{Got: secretValue, Want: []any{"label", "annotation", "env"}}, "does not match label, annotation or env"},
		{"enum with null", &kind.Enum{Got: secretValue, Want: []any{"by_name", nil}}, "does not match by_name or null"},
		{"enum with numbers", &kind.Enum{Got: 3, Want: []any{1, 2}}, "does not match 1 or 2"},
		{"const", &kind.Const{Got: secretValue, Want: "ebpfless"}, "must be ebpfless"},
		{"const null", &kind.Const{Got: secretValue, Want: nil}, "must be null"},
		{"pattern", &kind.Pattern{Got: secretValue, Want: "^https://"}, "does not match pattern ^https://"},
		{"format", &kind.Format{Got: secretValue, Want: "duration", Err: errors.New(secretValue)}, "is not a valid duration"},
		{"minimum", &kind.Minimum{Got: big.NewRat(-1, 1), Want: big.NewRat(0, 1)}, "must be >= 0"},
		{"maximum", &kind.Maximum{Got: big.NewRat(70000, 1), Want: big.NewRat(65535, 1)}, "must be <= 65535"},
		{"exclusive minimum", &kind.ExclusiveMinimum{Got: big.NewRat(0, 1), Want: big.NewRat(1, 2)}, "must be > 0.5"},
		{"exclusive maximum", &kind.ExclusiveMaximum{Got: big.NewRat(2, 1), Want: big.NewRat(1, 1)}, "must be < 1"},
		{"multiple of", &kind.MultipleOf{Got: big.NewRat(3, 1), Want: big.NewRat(2, 1)}, "must be a multiple of 2"},
		{"min length", &kind.MinLength{Got: 0, Want: 1}, "must be at least 1 characters long"},
		{"max length", &kind.MaxLength{Got: 300, Want: 255}, "must be at most 255 characters long"},
		{"min items", &kind.MinItems{Got: 0, Want: 1}, "must have at least 1 items"},
		{"max items", &kind.MaxItems{Got: 4, Want: 3}, "must have at most 3 items"},
		{"min properties", &kind.MinProperties{Got: 0, Want: 1}, "must have at least 1 properties"},
		{"max properties", &kind.MaxProperties{Got: 3, Want: 2}, "must have at most 2 properties"},
		{"unique items", &kind.UniqueItems{Duplicates: [2]int{0, 1}}, "items must be unique"},
		{"required", &kind.Required{Missing: []string{"port", "host"}}, "missing required properties host, port"},
		{"additional properties", &kind.AdditionalProperties{Properties: []string{secretValue}}, "contains unknown properties"},
		{"property names", &kind.PropertyNames{Property: secretValue}, "contains an invalid property name"},
		{"false schema", &kind.FalseSchema{}, "is not allowed"},
		{"other kind", &kind.Contains{}, "does not satisfy 'contains'"},
		{"other kind without keyword", &kind.Not{}, "is invalid"},
		{"no kind", nil, "is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := violationReason(tc.kind)
			assert.Equal(t, tc.want, reason)
			assert.NotContains(t, reason, secretValue)
		})
	}
}

func TestViolationReason_DoesNotSortRequiredInPlace(t *testing.T) {
	missing := []string{"port", "host"}
	violationReason(&kind.Required{Missing: missing})
	assert.Equal(t, []string{"port", "host"}, missing)
}

// TestViolationReasonFromSchema checks the kinds produced by the schema library are the ones violationReason expects
func TestViolationReasonFromSchema(t *testing.T) {
	const testSchema = `
properties:
  api_key:
    type: string
  port:
    type: integer
    minimum: 1
    maximum: 65535
  log_format:
    type: string
    enum: [json, text]
  hostname:
    type: string
    maxLength: 3
  proxy:
    type: string
    pattern: "^https://"
  section:
    type: object
    additionalProperties: false
    required: [enabled]
    properties:
      enabled:
        type: boolean
`
	var rawSchema interface{}
	require.NoError(t, yaml.Unmarshal([]byte(testSchema), &rawSchema))
	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("reason_schema", rawSchema))
	sch, err := c.Compile("reason_schema")
	require.NoError(t, err)

	for _, tc := range []struct {
		name   string
		config map[string]interface{}
		want   string
	}{
		{"type", map[string]interface{}{"api_key": 1234}, "got number instead of string"},
		{"minimum", map[string]interface{}{"port": 0}, "must be >= 1"},
		{"maximum", map[string]interface{}{"port": 70000}, "must be <= 65535"},
		{"enum", map[string]interface{}{"log_format": secretValue}, "does not match json or text"},
		{"max length", map[string]interface{}{"hostname": secretValue}, "must be at most 3 characters long"},
		{"pattern", map[string]interface{}{"proxy": secretValue}, "does not match pattern ^https://"},
		{"required", map[string]interface{}{"section": map[string]interface{}{}}, "missing required properties enabled"},
		{"additional properties", map[string]interface{}{"section": map[string]interface{}{"enabled": true, secretValue: 1}}, "contains unknown properties"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violations, err := validateData(sch, nil, tc.config, false)
			require.NoError(t, err)
			require.Len(t, violations, 1)
			assert.Equal(t, tc.want, violations[0].Reason)
			assert.NotContains(t, violations[0].Reason, secretValue)
		})
	}
}

func TestJoinOr(t *testing.T) {
	assert.Equal(t, "", joinOr(nil))
	assert.Equal(t, "a", joinOr([]string{"a"}))
	assert.Equal(t, "a or b", joinOr([]string{"a", "b"}))
	assert.Equal(t, "a, b or c", joinOr([]string{"a", "b", "c"}))
}

func TestSchemaValue(t *testing.T) {
	assert.Equal(t, "null", schemaValue(nil))
	assert.Equal(t, "text", schemaValue("text"))
	assert.Equal(t, "42", schemaValue(42))
	assert.Equal(t, "true", schemaValue(true))
}

func TestRatString(t *testing.T) {
	assert.Equal(t, "?", ratString(nil))
	assert.Equal(t, "0", ratString(big.NewRat(0, 1)))
	assert.Equal(t, "-5", ratString(big.NewRat(-5, 1)))
	assert.Equal(t, "0.25", ratString(big.NewRat(1, 4)))
	assert.Equal(t, "65535", ratString(big.NewRat(65535, 1)))
}
