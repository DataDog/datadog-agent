// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// powershell_helpers.go contains the platform-independent logic for transforming
// and evaluating PowerShell script templates.  Keeping it free of OS-specific
// imports allows the functions to be unit-tested on all platforms.

package com_datadoghq_script

import (
	"encoding/json"
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
	// For inline scripts: the script body with a param() block prepended.
	// ScriptArgs holds the corresponding named-parameter arguments.
	Script     string
	ScriptArgs []string // ["-__par_name", "Alice", "-__par_city", "NYC", ...]

	// For file-based scripts
	File      string
	Arguments []string
}

// transformInlineScript rewrites a PowerShell script template so that
// {{ parameters.X.Y.Z }} expressions are replaced by PowerShell variable
// references ($__par_X_Y_Z) and a matching param() block is prepended.
// The resolved parameter values are returned in ScriptArgs as alternating
// "-varName" / "value" pairs for use with powershell.exe -Command { ... }.
//
// This separates code (the script text) from data (user-supplied parameter
// values) at the OS argument level, eliminating template-injection attacks.
func transformInlineScript(scriptTemplate string, parameters interface{}) (*evaluatedPowershellScript, error) {
	parsed, err := tmpl.Parse(scriptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse script template: %w", err)
	}

	type paramEntry struct {
		path    []string // full path including "parameters" root, e.g. ["parameters","addr","city"]
		varName string   // PowerShell variable name, e.g. "__par_addr_city"
	}

	// Collect unique parameter references in order of first appearance.
	seen := make(map[string]*paramEntry)
	var order []string

	for _, path := range parsed.Expressions() {
		if len(path) < 2 || path[0] != "parameters" {
			// Non-parameters expressions are not supported for inline PowerShell scripts;
			// they will be rendered as empty strings to match pre-existing behaviour.
			continue
		}
		depth := len(path) - 1 // levels below "parameters"
		if depth > maxParameterDepth {
			return nil, fmt.Errorf(
				"parameter path %q exceeds the maximum supported nesting depth of %d",
				strings.Join(path, "."), maxParameterDepth,
			)
		}
		varName := pathToVarName(path)
		if _, exists := seen[varName]; !exists {
			seen[varName] = &paramEntry{path: path, varName: varName}
			order = append(order, varName)
		}
	}

	// Rewrite the script body: replace every {{ parameters.X }} with $__par_X.
	transformedBody, err := parsed.RenderWith(func(path []string) (string, error) {
		if len(path) >= 2 && path[0] == "parameters" {
			return "$" + pathToVarName(path), nil
		}
		return "", nil // non-parameters expressions → empty string
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rewrite script template: %w", err)
	}

	// Prepend a param() block so PowerShell binds named arguments to variables.
	// Each parameter defaults to $null so missing parameters don't cause errors.
	var script string
	if len(order) > 0 {
		decls := make([]string, len(order))
		for i, varName := range order {
			decls[i] = "    $" + varName + " = $null"
		}
		script = "param(\n" + strings.Join(decls, ",\n") + "\n)\n" + transformedBody
	} else {
		script = transformedBody
	}

	// Resolve parameter values and build the named-argument list.
	var scriptArgs []string
	for _, varName := range order {
		entry := seen[varName]
		// path[1:] strips the "parameters" root; EvaluatePathParts traverses into parameters.
		val, err := tmpl.EvaluatePathParts(parameters, entry.path[1:])
		if err != nil || val == nil {
			// Parameter not provided — the $null default in the param() block applies.
			continue
		}
		strVal, err := serializeParamValue(val)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize parameter %q: %w",
				strings.Join(entry.path, "."), err)
		}
		scriptArgs = append(scriptArgs, "-"+varName, strVal)
	}

	return &evaluatedPowershellScript{
		Script:     script,
		ScriptArgs: scriptArgs,
	}, nil
}

// pathToVarName converts a parameter path (rooted at "parameters") to a safe
// PowerShell variable name.
//
//	["parameters", "name"]            → "__par_name"
//	["parameters", "address", "city"] → "__par_address_city"
//	["parameters", "items", "0"]      → "__par_items_0"
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

// serializeParamValue converts a Go parameter value to a string suitable for
// passing as a PowerShell scriptblock named argument.
// Strings are passed as-is (the OS argument boundary prevents any injection).
// Numbers and booleans are formatted as plain strings.
// Objects and arrays are JSON-encoded so the script can use ConvertFrom-Json.
func serializeParamValue(val interface{}) (string, error) {
	switch v := val.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case float64:
		// JSON unmarshaling produces float64 for all numbers.
		// Emit as integer when the value is whole to avoid "42.000000" noise.
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		// Objects, arrays, and any other types are JSON-encoded.
		b, err := json.Marshal(val)
		if err != nil {
			return "", fmt.Errorf("failed to JSON-encode value: %w", err)
		}
		return string(b), nil
	}
}

