// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// maxWhereEntries bounds the Where-Object stages one instance may emit, because
// CreateProcess caps a command line at 32767 characters.
const maxWhereEntries = 32

var (
	// cmdletNameRegex matches a read-only Get-* cmdlet name. Still load-bearing:
	// Get-Command -Name accepts wildcards, and this keeps a pattern out of it.
	cmdletNameRegex = regexp.MustCompile(`^Get-[A-Za-z0-9]+$`)

	// identifierRegex matches a parameter or property identifier. These travel in
	// the payload, so this is a sanity gate rather than an injection defence.
	identifierRegex = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// validateGetCmdletName verifies name is a syntactically valid read-only Get-* cmdlet.
func validateGetCmdletName(name string) error {
	if !cmdletNameRegex.MatchString(name) {
		return fmt.Errorf("cmdlet %q is not a read-only Get-* cmdlet (must match Get-<Noun>)", name)
	}
	return nil
}

// validateIdentifier verifies a parameter or property name is a safe identifier.
func validateIdentifier(kind, name string) error {
	if !identifierRegex.MatchString(name) {
		return fmt.Errorf("%s %q contains characters outside [A-Za-z0-9_]", kind, name)
	}
	return nil
}

// scriptPayload is the data half of a command invocation: everything derived from
// configuration, sent as JSON on the child's stdin, never rendered into source.
type scriptPayload struct {
	// Lower-case tags are load-bearing; the generated script reads these names.
	Cmdlet     string         `json:"cmdlet"`
	Module     string         `json:"module"`
	Parameters []payloadParam `json:"parameters"`
	Where      []payloadWhere `json:"where"`
	Select     []string       `json:"select"`
}

// payloadParam is one named cmdlet parameter, splatted to the cmdlet as data.
type payloadParam struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

// payloadWhere is one filter entry. The operator is not carried here: it comes
// from the closed whereOps map, so it can never originate in configuration.
type payloadWhere struct {
	Property string      `json:"property"`
	Value    interface{} `json:"value"`
}

// scriptPreamble is the fixed head of every command and holds no configuration.
// Preserve: reads stdin to EOF, explicit UTF-8 both ways, no double-quote character.
const scriptPreamble = `$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)
$sr  = New-Object IO.StreamReader([Console]::OpenStandardInput(), (New-Object Text.UTF8Encoding($false)))
$cfg = $sr.ReadToEnd() | ConvertFrom-Json

$c = Get-Command -Name $cfg.cmdlet -CommandType Cmdlet,Function -ErrorAction Stop
if (@($c).Count -ne 1) { throw 'powershell check: name resolved to multiple commands' }
if ($c.Verb -ne 'Get') { throw 'powershell check: not a read-only Get- cmdlet' }
`

// buildPayload assembles and validates the data half of a command invocation,
// kept separate so value handling can be tested without going near script text.
func buildPayload(cmdlet, module string, params []parameterEntry, where []whereEntry, selectProps []string) (scriptPayload, error) {
	var p scriptPayload

	if err := validateGetCmdletName(cmdlet); err != nil {
		return p, err
	}
	if len(where) > maxWhereEntries {
		return p, fmt.Errorf("too many 'where' entries: %d (maximum %d)", len(where), maxWhereEntries)
	}

	// Normalized to empty rather than nil so the payload shape is stable.
	p = scriptPayload{
		Cmdlet:     cmdlet,
		Module:     module,
		Parameters: make([]payloadParam, 0, len(params)),
		Where:      make([]payloadWhere, 0, len(where)),
		Select:     make([]string, 0, len(selectProps)),
	}

	seen := make(map[string]struct{}, len(params))
	for i := range params {
		if err := validateIdentifier("parameter", params[i].Name); err != nil {
			return scriptPayload{}, err
		}
		// PowerShell hashtable keys are case-insensitive and the splat table is
		// built by assignment, so a duplicate would silently overwrite.
		key := strings.ToLower(params[i].Name)
		if _, dup := seen[key]; dup {
			return scriptPayload{}, fmt.Errorf("parameter %q is specified more than once", params[i].Name)
		}
		seen[key] = struct{}{}
		p.Parameters = append(p.Parameters, payloadParam{Name: params[i].Name, Value: params[i].Value})
	}

	for i := range where {
		// Re-checked here so a hand-constructed entry cannot bypass finalize.
		if err := validateIdentifier("property", where[i].Property); err != nil {
			return scriptPayload{}, fmt.Errorf("where entry: %w", err)
		}
		p.Where = append(p.Where, payloadWhere{Property: where[i].Property, Value: where[i].Value})
	}

	for i := range selectProps {
		if err := validateIdentifier("property", selectProps[i]); err != nil {
			return scriptPayload{}, err
		}
		p.Select = append(p.Select, selectProps[i])
	}

	return p, nil
}

// buildCommand renders the PowerShell script plus the JSON payload carrying every
// configuration-derived value. The script contains NO configuration text: only a
// stage index and an operator switch/token. See TestScriptIsValueIndependent.
func buildCommand(cmdlet, module string, params []parameterEntry, where []whereEntry, selectProps []string) (string, []byte, error) {
	payload, err := buildPayload(cmdlet, module, params, where, selectProps)
	if err != nil {
		return "", nil, err
	}

	// Rendered up front so a bad entry fails the build, not a half-written script.
	var decls, pipe strings.Builder
	for i := range where {
		if err := where[i].writeStage(&decls, &pipe, i); err != nil {
			return "", nil, err
		}
	}

	var b strings.Builder
	b.WriteString(scriptPreamble)
	// Rejects a same-named function shadowing the cmdlet from another module. "*"
	// opts out; whether to emit the guard stays a Go-side decision.
	if module != "" && module != "*" {
		b.WriteString("if ($c.ModuleName -ne $cfg.module) { throw 'powershell check: cmdlet resolved to an unexpected module' }\n")
	}
	b.WriteString("\n$p = @{}\nforeach ($e in $cfg.parameters) { $p[$e.name] = $e.value }\n")
	// Resolved rather than invoked by bare name, which another module could shadow.
	if len(where) > 0 {
		b.WriteString("$w = Get-Command -Name 'Where-Object' -CommandType Cmdlet -ErrorAction Stop\n")
	}
	b.WriteString(decls.String())

	// -InputObject @(...) forces a JSON array for 0, 1 or N rows, where piping would
	// unroll it. Invoke $c, the object validated above, rather than the name.
	b.WriteString("ConvertTo-Json -Depth 8 -Compress -InputObject @(& $c @p")
	// Filter before projecting, or Select-Object drops the properties being tested.
	b.WriteString(pipe.String())
	// Select-Object -Property throws on an empty list, and selectProperties returns
	// nothing when every metric is virtual with no tag_by or tag_queries.
	if len(selectProps) > 0 {
		b.WriteString(" | Select-Object -Property $cfg.select")
	}
	b.WriteString(")\n")

	data, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("could not marshal command payload: %w", err)
	}
	return b.String(), data, nil
}
