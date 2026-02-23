// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Parse / Render ---

// TestParseAndRender_PlainString verifies that a template with no expressions is
// returned verbatim.
func TestParseAndRender_PlainString(t *testing.T) {
	result, err := ParseAndRender("hello world", nil)
	require.NoError(t, err)
	assert.Equal(t, "hello world", result)
}

// TestParseAndRender_SingleExpression verifies that {{ key }} is replaced with the
// value of "key" from the input map.
func TestParseAndRender_SingleExpression(t *testing.T) {
	input := map[string]interface{}{"name": "alice"}
	result, err := ParseAndRender("Hello {{ name }}!", input)
	require.NoError(t, err)
	assert.Equal(t, "Hello alice!", result)
}

// TestParseAndRender_NestedPath verifies that dotted paths like {{ user.email }} are
// resolved recursively through nested maps.
func TestParseAndRender_NestedPath(t *testing.T) {
	input := map[string]interface{}{
		"user": map[string]interface{}{"email": "alice@example.com"},
	}
	result, err := ParseAndRender("email: {{ user.email }}", input)
	require.NoError(t, err)
	assert.Equal(t, "email: alice@example.com", result)
}

// TestParseAndRender_MissingKeyRendersEmpty verifies that when a key is absent from the
// input the expression renders to an empty string (no error), preserving the surrounding text.
func TestParseAndRender_MissingKeyRendersEmpty(t *testing.T) {
	input := map[string]interface{}{"other": "value"}
	result, err := ParseAndRender("prefix-{{ missing }}-suffix", input)
	require.NoError(t, err)
	assert.Equal(t, "prefix--suffix", result)
}

// TestParseAndRender_MultipleExpressions verifies that several {{ }} blocks in one
// template are all resolved independently.
func TestParseAndRender_MultipleExpressions(t *testing.T) {
	input := map[string]interface{}{"first": "foo", "second": "bar"}
	result, err := ParseAndRender("{{ first }} and {{ second }}", input)
	require.NoError(t, err)
	assert.Equal(t, "foo and bar", result)
}

// TestParse_UnexpectedToken verifies that a malformed template (e.g. using "..") returns
// a ParseError rather than silently producing a broken template.
func TestParse_UnexpectedToken(t *testing.T) {
	_, err := Parse("{{ .. }}")
	require.Error(t, err)
	var pe ParseError
	assert.ErrorAs(t, err, &pe)
}

// TestParse_EmptyExpression verifies that {{ }} (open then immediately close) returns an error.
func TestParse_EmptyExpression(t *testing.T) {
	_, err := Parse("{{ }}")
	require.Error(t, err)
}

// TestParse_SlashSeparatorForbidden verifies that a path separator of "/" is rejected
// because only "." is a valid path separator.
func TestParse_SlashSeparatorForbidden(t *testing.T) {
	_, err := Parse("{{ a/b }}")
	require.Error(t, err)
}

// --- PreserveExpressionsWithPathRoots ---

// TestParseAndRender_PreserveExpressionRoot verifies that when a path root is registered
// for preservation, the matching {{ }} expression is emitted as a literal string instead
// of being evaluated. This is used to pass through expressions destined for another
// template engine.
func TestParseAndRender_PreserveExpressionRoot(t *testing.T) {
	input := map[string]interface{}{"steps": "ignored"}
	opt := PreserveExpressionsWithPathRoots("steps")
	result, err := ParseAndRender("value: {{ steps.output }}", input, opt)
	require.NoError(t, err)
	assert.Equal(t, "value: {{ steps.output }}", result)
}

// TestParseAndRender_PreserveDoesNotAffectOtherRoots verifies that only the preserved
// root is kept literal; other roots are still evaluated normally.
func TestParseAndRender_PreserveDoesNotAffectOtherRoots(t *testing.T) {
	input := map[string]interface{}{"env": "production"}
	opt := PreserveExpressionsWithPathRoots("steps")
	result, err := ParseAndRender("{{ env }} {{ steps.x }}", input, opt)
	require.NoError(t, err)
	assert.Equal(t, "production {{ steps.x }}", result)
}

// --- EvaluatePath ---

// TestEvaluatePath_MapKey verifies basic map key lookup via a dotted path expression.
func TestEvaluatePath_MapKey(t *testing.T) {
	input := map[string]interface{}{"region": "us-east-1"}
	val, err := EvaluatePath(input, "region")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", val)
}

// TestEvaluatePath_SliceIndex verifies that numeric path segments select slice elements.
// The lexer does not accept bare digits in path expressions, so we call the private
// evaluatePath with a pre-built path slice — testing the slice-index branch directly.
func TestEvaluatePath_SliceIndex(t *testing.T) {
	input := map[string]interface{}{
		"tags": []interface{}{"alpha", "beta", "gamma"},
	}
	// Access through evaluatePath directly: ["tags", "1"] → second element of the slice.
	val, err := evaluatePath(input, []string{"tags", "1"})
	require.NoError(t, err)
	assert.Equal(t, "beta", val)
}

// TestEvaluatePath_StructField verifies that exported struct fields are accessible via
// their title-cased name. The evaluator capitalises the first letter before looking up
// the field.
func TestEvaluatePath_StructField(t *testing.T) {
	type Config struct {
		Host string
		Port int
	}
	input := Config{Host: "localhost", Port: 8080}
	val, err := EvaluatePath(input, "host")
	require.NoError(t, err)
	assert.Equal(t, "localhost", val)
}

// TestEvaluatePath_NotFound returns ErrPathNotFound so callers can distinguish a missing
// value from other errors.
func TestEvaluatePath_NotFound(t *testing.T) {
	input := map[string]interface{}{"a": "1"}
	_, err := EvaluatePath(input, "b")
	require.Error(t, err)
	var epnf ErrPathNotFound
	assert.ErrorAs(t, err, &epnf)
	assert.Contains(t, epnf.FullyQualifiedPath, "b")
}

// TestEvaluatePath_InvalidPathExpr verifies that a non-path expression string (e.g. one
// that won't parse as a valid {{ }} path) returns an error.
func TestEvaluatePath_InvalidPathExpr(t *testing.T) {
	_, err := EvaluatePath(map[string]interface{}{}, "..")
	require.Error(t, err)
}

// TestEvaluatePath_BracketedKey verifies that [key with spaces] syntax is unwrapped
// correctly so the lookup uses the bare key name.
func TestEvaluatePath_BracketedKey(t *testing.T) {
	input := map[string]interface{}{"key with spaces": "found"}
	val, err := EvaluatePath(input, "[key with spaces]")
	require.NoError(t, err)
	assert.Equal(t, "found", val)
}

// --- stringify ---

// TestStringify_String verifies that a string value passes through as-is.
func TestStringify_String(t *testing.T) {
	s, err := stringify("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", s)
}

// TestStringify_Int verifies that integer types are formatted as decimal strings.
func TestStringify_Int(t *testing.T) {
	s, err := stringify(42)
	require.NoError(t, err)
	assert.Equal(t, "42", s)
}

// TestStringify_Float verifies that float64 values use full precision without
// scientific notation (uses 'f' format).
func TestStringify_Float(t *testing.T) {
	s, err := stringify(float64(3.14))
	require.NoError(t, err)
	assert.Equal(t, "3.14", s)
}

// TestStringify_Map verifies that maps are JSON-serialised (without HTML escaping).
func TestStringify_Map(t *testing.T) {
	m := map[string]interface{}{"url": "http://example.com?a=1&b=2"}
	s, err := stringify(m)
	require.NoError(t, err)
	// HTML escaping must be disabled: & should NOT become \u0026
	assert.Contains(t, s, "&")
	assert.NotContains(t, s, `\u0026`)
}

// TestStringify_Slice verifies that slices are JSON-serialised.
func TestStringify_Slice(t *testing.T) {
	s, err := stringify([]int{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, "[1,2,3]", s)
}

// TestStringify_NilValue verifies that an untyped nil produces an empty string rather
// than "null" or a panic.
func TestStringify_NilValue(t *testing.T) {
	s, err := stringify(nil)
	require.NoError(t, err)
	assert.Equal(t, "", s)
}

// --- MustEvaluatePath ---

// TestMustEvaluatePath_PanicsOnMissingKey verifies that MustEvaluatePath panics when
// the path is not found, as documented.
func TestMustEvaluatePath_PanicsOnMissingKey(t *testing.T) {
	assert.Panics(t, func() {
		MustEvaluatePath(map[string]interface{}{}, "missing")
	})
}

// TestMustEvaluatePath_ReturnsValueOnSuccess verifies the happy path: a found value is
// returned directly.
func TestMustEvaluatePath_ReturnsValueOnSuccess(t *testing.T) {
	input := map[string]interface{}{"key": "value"}
	assert.NotPanics(t, func() {
		result := MustEvaluatePath(input, "key")
		assert.Equal(t, "value", result)
	})
}
