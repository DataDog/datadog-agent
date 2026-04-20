// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateExpression_Valid(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		eventType string
		fields    []string
	}{
		{
			name:      "simple field comparison",
			expr:      `process.file.name == "curl"`,
			eventType: "",
			fields:    []string{"process.file.name"},
		},
		{
			name:      "open event with path",
			expr:      `open.file.path == "/etc/passwd"`,
			eventType: "open",
			fields:    []string{"open.file.path"},
		},
		{
			name:      "compound expression",
			expr:      `open.file.path == "/etc/shadow" && process.file.name == "cat"`,
			eventType: "open",
			fields:    []string{"open.file.path", "process.file.name"},
		},
		{
			name:      "in array",
			expr:      `exec.file.path in ["/usr/bin/curl", "/usr/bin/wget"]`,
			eventType: "exec",
			fields:    []string{"exec.file.path"},
		},
		{
			name:      "integer comparison",
			expr:      `process.uid == 0`,
			eventType: "",
			fields:    []string{"process.uid"},
		},
		{
			name:      "boolean field",
			expr:      `process.is_thread`,
			eventType: "",
			fields:    []string{"process.is_thread"},
		},
		{
			name:      "negation",
			expr:      `process.file.name != "sshd"`,
			eventType: "",
			fields:    []string{"process.file.name"},
		},
		{
			name:      "dns event",
			expr:      `dns.question.type == A`,
			eventType: "dns",
			fields:    []string{"dns.question.type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateExpression(tt.expr)
			assert.True(t, result.Valid, "expression should be valid: %s", tt.expr)
			assert.Empty(t, result.Diagnostics)
			assert.Equal(t, tt.eventType, result.EventType)
			assert.ElementsMatch(t, tt.fields, result.Fields)
		})
	}
}

func TestValidateExpression_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		errContains string
		field       string
	}{
		{
			name:        "unknown field",
			expr:        `process.bogusfield == "test"`,
			errContains: "field `process.bogusfield` not found",
			field:       "process.bogusfield",
		},
		{
			name:        "type mismatch int vs string",
			expr:        `process.uid == "root"`,
			errContains: "int expected",
		},
		{
			name:        "syntax error missing rhs",
			expr:        `process.file.name ==`,
			errContains: "parse error",
		},
		{
			name:        "syntax error bad operator",
			expr:        `process.file.name === "test"`,
			errContains: "parse error",
		},
		{
			name:        "cross event types",
			expr:        `open.file.path == "/etc/passwd" && exec.file.path == "/usr/bin/cat"`,
			errContains: "multiple event types",
		},
		{
			name:        "empty expression",
			expr:        ``,
			errContains: "parse error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateExpression(tt.expr)
			assert.False(t, result.Valid, "expression should be invalid: %s", tt.expr)
			require.NotEmpty(t, result.Diagnostics)
			assert.Equal(t, "error", result.Diagnostics[0].Severity)
			assert.Contains(t, result.Diagnostics[0].Message, tt.errContains)
			if tt.field != "" {
				assert.Equal(t, tt.field, result.Diagnostics[0].Field)
			}
		})
	}
}

func TestValidateExpression_UnknownFieldHasSuggestions(t *testing.T) {
	result := validateExpression(`process.bogusfield == "test"`)
	require.NotEmpty(t, result.Diagnostics)
	assert.NotEmpty(t, result.Diagnostics[0].Suggestions, "unknown field should produce suggestions")
}

func TestSuggestFields_ExactPrefix(t *testing.T) {
	suggestions := suggestFields("process.file.name", "")
	require.NotEmpty(t, suggestions)
	assert.Equal(t, "process.file.name", suggestions[0])
}

func TestSuggestFields_Typo(t *testing.T) {
	suggestions := suggestFields("process.file.nme", "")
	require.NotEmpty(t, suggestions)
	assert.Equal(t, "process.file.name", suggestions[0], "should correct single-char typo")
}

func TestSuggestFields_PartialPath(t *testing.T) {
	suggestions := suggestFields("open.file", "")
	require.NotEmpty(t, suggestions)
	for _, s := range suggestions {
		assert.Contains(t, s, "open.file")
	}
}

func TestSuggestFields_FilterByEventType(t *testing.T) {
	suggestions := suggestFields("file.path", "exec")
	require.NotEmpty(t, suggestions)

	allFields := getAllFields()
	fieldMap := make(map[string]FieldInfo)
	for _, f := range allFields {
		fieldMap[f.Name] = f
	}
	for _, s := range suggestions {
		fi, ok := fieldMap[s]
		require.True(t, ok)
		if fi.EventType != "" {
			assert.Equal(t, "exec", fi.EventType, "suggestion %s has wrong event type", s)
		}
	}
}

func TestGetAllFields(t *testing.T) {
	fields := getAllFields()
	assert.Greater(t, len(fields), 100, "should have many fields")

	seen := make(map[string]bool)
	for _, f := range fields {
		assert.NotEmpty(t, f.Name)
		assert.NotEmpty(t, f.Type)
		assert.False(t, seen[f.Name], "duplicate field: %s", f.Name)
		seen[f.Name] = true
	}
}

func TestGetAllFields_Sorted(t *testing.T) {
	fields := getAllFields()
	for i := 1; i < len(fields); i++ {
		assert.LessOrEqual(t, fields[i-1].Name, fields[i].Name, "fields should be sorted")
	}
}

func TestListFields_FilterByEventType(t *testing.T) {
	result := listFields("exec")
	require.NotEmpty(t, result.Fields)

	for _, f := range result.Fields {
		if f.EventType != "" {
			assert.Equal(t, "exec", f.EventType, "field %s should belong to exec or be global", f.Name)
		}
	}
}

func TestListFields_NoFilter(t *testing.T) {
	all := listFields("")
	exec := listFields("exec")
	assert.Greater(t, len(all.Fields), len(exec.Fields), "unfiltered should have more fields")
}

func TestListEventTypes(t *testing.T) {
	result := listEventTypes()
	require.NotEmpty(t, result.EventTypes)
	assert.Contains(t, result.EventTypes, "exec")
	assert.Contains(t, result.EventTypes, "open")
	assert.Contains(t, result.EventTypes, "dns")

	// should be sorted
	for i := 1; i < len(result.EventTypes); i++ {
		assert.LessOrEqual(t, result.EventTypes[i-1], result.EventTypes[i])
	}
}

func TestSerializableConstants(t *testing.T) {
	consts := serializableConstants()
	// On darwin, Linux-specific constants (O_WRONLY etc.) won't be present,
	// but common ones (DNS types, architectures) should be.
	assert.NotEmpty(t, consts, "should have at least some constants")

	for k, v := range consts {
		assert.NotEmpty(t, k)
		switch v.(type) {
		case int, string, bool, float64, []string, []int, []bool:
			// ok
		default:
			t.Errorf("constant %s has non-serializable type %T", k, v)
		}
	}
}

func TestCloseEnough(t *testing.T) {
	tests := []struct {
		a, b   string
		expect bool
	}{
		{"name", "name", true},
		{"name", "nme", true},   // 1 deletion
		{"name", "naem", true},  // 1 transposition-ish (edit dist 2)
		{"name", "nmae", true},  //nolint:misspell // edit dist 2
		{"name", "xyz", false},  // too different
		{"name", "n", false},    // too short
		{"file", "flie", true},  // swap
		{"file", "field", true}, // edit dist 2
		{"", "", true},
		{"a", "", true}, // edit dist 1
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := closeEnough(tt.a, tt.b)
			assert.Equal(t, tt.expect, got, "closeEnough(%q, %q)", tt.a, tt.b)
		})
	}
}

func TestSuggestFieldsCmd(t *testing.T) {
	result := suggestFieldsCmd("process.file.name", "")
	require.NotEmpty(t, result.Fields)
	assert.Equal(t, "process.file.name", result.Fields[0].Name)
	assert.NotEmpty(t, result.Fields[0].Type)
}
