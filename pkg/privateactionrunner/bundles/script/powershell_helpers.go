// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// powershell_helpers.go contains the platform-independent logic for transforming
// and evaluating PowerShell script templates.  Keeping it free of OS-specific
// imports allows the functions to be unit-tested on all platforms.

package com_datadoghq_script

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/tmpl"
)

// maxParameterDepth is the maximum number of nesting levels supported for
// {{ parameters.a.b.c.d.e }} expressions (5 levels below the "parameters" root).
const maxParameterDepth = 5

// evaluatedPowershellScript holds the evaluated script configuration ready for execution.
type evaluatedPowershellScript struct {
	// For inline scripts: complete script body, with a safe variable-assignment
	// preamble prepended for each {{ parameters.X }} reference.
	Script string

	// For file-based scripts
	File      string
	Arguments []string
}

// transformInlineScript rewrites a PowerShell script template so that every
// {{ parameters.X.Y.Z }} expression is replaced by a PowerShell variable
// reference ($__par_X_Y_Z).  A safe variable-assignment preamble is prepended
// to the script body, binding each variable to its resolved value as a
// PowerShell single-quoted string literal.
//
// Single-quoted strings in PowerShell expand no variables and process no escape
// sequences; the only special case is that a literal single quote inside the
// string is written as two consecutive single quotes. This means user-supplied
// values — regardless of whether they contain $, `, ;, backslashes, newlines,
// or any other character — can never break out of the assignment and inject
// arbitrary PowerShell code.
func transformInlineScript(scriptTemplate string, parameters any) (*evaluatedPowershellScript, error) {
	parsed, err := tmpl.Parse(scriptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse script template: %w", err)
	}

	type paramEntry struct {
		path    []string // full path including "parameters" root
		varName string   // PowerShell variable name, e.g. "__par_addr_city"
	}

	// Collect unique parameter references in order of first appearance.
	// seenPaths deduplicates identical expressions (same path used multiple times).
	// entries maps varName → entry and is used to detect collisions where two
	// distinct paths (e.g. parameters.foo-bar and parameters.foo_bar) produce the
	// same sanitized PowerShell variable name.
	seenPaths := make(map[string]bool)
	entries := make(map[string]*paramEntry)
	var order []string

	for _, path := range parsed.Expressions() {
		if len(path) == 0 || path[0] != "parameters" {
			// Non-parameters expressions are not supported for inline PowerShell scripts;
			// they will be rendered as empty strings to match pre-existing behaviour.
			continue
		}
		if len(path) == 1 {
			// {{ parameters }} refers to the entire parameters object. This cannot be
			// safely assigned to a single variable in the preamble approach. Reference
			// individual fields instead, e.g. {{ parameters.fieldName }}.
			return nil, errors.New(
				"{{ parameters }} is not supported in inline scripts; reference individual fields using {{ parameters.fieldName }}",
			)
		}
		depth := len(path) - 1 // levels below "parameters"
		if depth > maxParameterDepth {
			return nil, fmt.Errorf(
				"parameter path %q exceeds the maximum supported nesting depth of %d",
				strings.Join(path, "."), maxParameterDepth,
			)
		}
		fullPath := strings.Join(path, ".")
		if seenPaths[fullPath] {
			continue // same expression used more than once — already handled
		}
		seenPaths[fullPath] = true

		varName := pathToVarName(path)
		if existing, collision := entries[varName]; collision {
			return nil, fmt.Errorf(
				"parameters %q and %q both map to PowerShell variable $%s; rename one to avoid the collision",
				strings.Join(existing.path, "."), fullPath, varName,
			)
		}
		entries[varName] = &paramEntry{path: path, varName: varName}
		order = append(order, varName)
	}

	// Rewrite the script body: replace every {{ parameters.X }} with $__par_X.
	// The script body itself contains only static code and variable references —
	// never raw user-supplied data.
	transformedBody, err := parsed.RenderWith(func(path []string) (string, error) {
		if len(path) >= 2 && path[0] == "parameters" {
			return "$" + pathToVarName(path), nil
		}
		if len(path) == 1 && path[0] == "parameters" {
			return "", errors.New(
				"{{ parameters }} is not supported in inline scripts; reference individual fields using {{ parameters.fieldName }}",
			)
		}
		return "", nil // non-parameters expressions → empty string
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rewrite script template: %w", err)
	}

	// Build the preamble: one safe variable assignment per parameter.
	// Values are encoded as PowerShell single-quoted string literals, which
	// prevents injection regardless of the value's content.
	preamble := make([]string, 0, len(order))
	for _, varName := range order {
		entry := entries[varName]
		// path[1:] strips the "parameters" root.
		val, err := tmpl.EvaluatePathParts(parameters, entry.path[1:])
		if err != nil {
			var notFound tmpl.ErrPathNotFound
			if !errors.As(err, &notFound) {
				return nil, fmt.Errorf("failed to evaluate parameter %q: %w", strings.Join(entry.path, "."), err)
			}
			// Parameter not provided — assign $null so the variable exists.
			preamble = append(preamble, "$"+varName+" = $null")
			continue
		}
		if val == nil {
			preamble = append(preamble, "$"+varName+" = $null")
			continue
		}
		literal, err := powershellLiteral(val)
		if err != nil {
			return nil, fmt.Errorf("failed to encode parameter %q as PowerShell literal: %w",
				strings.Join(entry.path, "."), err)
		}
		preamble = append(preamble, "$"+varName+" = "+literal)
	}

	var script string
	if len(preamble) > 0 {
		script = strings.Join(preamble, "\n") + "\n" + transformedBody
	} else {
		script = transformedBody
	}

	return &evaluatedPowershellScript{Script: script}, nil
}

// powershellLiteral converts a Go value to a PowerShell literal expression that
// is safe to embed directly in a script.
//
//   - Strings are wrapped in single quotes with ' escaped as ”.
//     Single-quoted strings have no other special characters, so no further
//     escaping is needed regardless of the string's content.
//   - Booleans become $true / $false.
//   - Numbers become unquoted numeric literals (validated by the type switch).
//   - Objects and arrays are JSON-encoded and then wrapped in single quotes;
//     the script can convert them with  $var | ConvertFrom-Json  if needed.
func powershellLiteral(val any) (string, error) {
	switch v := val.(type) {
	case nil:
		return "$null", nil
	case bool:
		if v {
			return "$true", nil
		}
		return "$false", nil
	case float64:
		// JSON unmarshaling produces float64 for all numbers.
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case string:
		return singleQuote(v), nil
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(val); err != nil {
			return "", fmt.Errorf("failed to JSON-encode value: %w", err)
		}
		// json.Encoder.Encode appends a trailing newline; strip it.
		return singleQuote(strings.TrimRight(buf.String(), "\n")), nil
	}
}

// singleQuote wraps s in PowerShell single quotes, escaping any single quotes
// within s by doubling them.  This is the only escaping required for
// single-quoted strings in PowerShell.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// pathToVarName converts a parameter path (rooted at "parameters") to a safe
// PowerShell variable name.
//
//	["parameters", "name"]            → "__par_name"
//	["parameters", "address", "city"] → "__par_address_city"
//	["parameters", "items", "[0]"]    → "__par_items__0_"
func pathToVarName(path []string) string {
	parts := make([]string, len(path)-1)
	for i, p := range path[1:] {
		parts[i] = sanitizeVarPart(p)
	}
	return "__par_" + strings.Join(parts, "_")
}

// sanitizeVarPart replaces characters that are not valid in a PowerShell
// variable name (letters, digits, underscores) with underscores.
func sanitizeVarPart(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	if sb.Len() == 0 {
		return "_"
	}
	return sb.String()
}
