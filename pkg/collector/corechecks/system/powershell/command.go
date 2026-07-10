// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// cmdletNameRegex matches a read-only Get-* cmdlet name. Restricting the
	// character set means the name is always safe to embed (single-quoted) in
	// the generated command.
	cmdletNameRegex = regexp.MustCompile(`^Get-[A-Za-z0-9]+$`)

	// identifierRegex matches a PowerShell parameter or property identifier.
	// Only these validated identifiers are ever placed in command positions.
	identifierRegex = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// validateGetCmdletName verifies that name is a syntactically valid, read-only
// Get-* cmdlet name.
func validateGetCmdletName(name string) error {
	if !cmdletNameRegex.MatchString(name) {
		return fmt.Errorf("cmdlet %q is not a read-only Get-* cmdlet (must match Get-<Noun>)", name)
	}
	return nil
}

// validateIdentifier verifies that a parameter or property name is a safe
// identifier suitable for embedding in the generated command.
func validateIdentifier(kind, name string) error {
	if !identifierRegex.MatchString(name) {
		return fmt.Errorf("%s %q contains characters outside [A-Za-z0-9_]", kind, name)
	}
	return nil
}

// buildCommand renders the fixed-shape PowerShell command that invokes a
// validated Get-* cmdlet with bound (splatted) parameters, projects the given
// properties, and emits a compact JSON array.
//
// Security: the cmdlet name and every parameter/property name are validated
// identifiers; every parameter *value* is encoded as a PowerShell single-quoted
// literal via powershellLiteral, so hostile values can never escape into an
// executable position. A verb re-check runs inside the command as defense in
// depth.
func buildCommand(cmdlet string, filters []filterEntry, selectProps []string) (string, error) {
	if err := validateGetCmdletName(cmdlet); err != nil {
		return "", err
	}
	for i := range selectProps {
		if err := validateIdentifier("property", selectProps[i]); err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString("$ErrorActionPreference = 'Stop'\n")
	// Defense in depth: resolve the command and re-check the verb at runtime.
	fmt.Fprintf(&b, "$c = Get-Command -Name %s -ErrorAction Stop\n", singleQuote(cmdlet))
	b.WriteString("if ($c.Verb -ne 'Get') { throw 'powershell check: not a read-only Get- cmdlet' }\n")

	// Build the splat table. Keys are validated identifiers; values are safe literals.
	b.WriteString("$p = @{")
	for i := range filters {
		if err := validateIdentifier("parameter", filters[i].Name); err != nil {
			return "", err
		}
		lit, err := powershellLiteral(filters[i].Value)
		if err != nil {
			return "", fmt.Errorf("filter %q: %w", filters[i].Name, err)
		}
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s = %s", filters[i].Name, lit)
	}
	b.WriteString("}\n")

	// Pass the results as -InputObject rather than piping: piping an array into
	// ConvertTo-Json unrolls it, so a single row would serialize as a bare object.
	// -InputObject @(...) reliably emits a JSON array for 0, 1, or N rows in
	// Windows PowerShell 5.1 (which has no ConvertTo-Json -AsArray).
	fmt.Fprintf(&b, "ConvertTo-Json -Depth 8 -Compress -InputObject @(& %s @p", singleQuote(cmdlet))
	if len(selectProps) > 0 {
		fmt.Fprintf(&b, " | Select-Object %s", strings.Join(selectProps, ","))
	}
	b.WriteString(")\n")

	return b.String(), nil
}

// powershellLiteral converts a Go value into a PowerShell literal that is safe
// to embed directly in a command. Adapted from the Private Action Runner's
// injection-safe encoder (pkg/privateactionrunner/bundles/script).
//
//   - Strings are single-quoted with ' doubled (the only escaping single-quoted
//     PowerShell strings require), so any content is inert.
//   - Booleans become $true / $false.
//   - Numbers become unquoted numeric literals.
//   - Anything else is JSON-encoded then single-quoted.
func powershellLiteral(val interface{}) (string, error) {
	switch v := val.(type) {
	case nil:
		return "$null", nil
	case bool:
		if v {
			return "$true", nil
		}
		return "$false", nil
	case string:
		return singleQuote(v), nil
	case int:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(val); err != nil {
			return "", fmt.Errorf("failed to JSON-encode value: %w", err)
		}
		return singleQuote(strings.TrimRight(buf.String(), "\n")), nil
	}
}

// singleQuote wraps s in PowerShell single quotes, doubling any embedded single
// quotes. This is the only escaping single-quoted PowerShell strings require.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
