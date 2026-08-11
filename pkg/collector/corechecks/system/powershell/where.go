// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"errors"
	"fmt"
	"strings"
)

// whereOpKind selects how an operator's comparison is rendered. Every kind
// produces a total expression: comparing a missing, null, or wrongly-typed
// property must drop the row, never throw. buildCommand runs the whole script
// under $ErrorActionPreference = 'Stop', so a throwing comparison would fail the
// entire check run, including metrics that have nothing to do with the filter.
type whereOpKind int

const (
	// whereOpString compares the property directly. PowerShell stringifies the
	// left-hand side for these operators, so they are already total.
	whereOpString whereOpKind = iota
	// whereOpCastString compares an explicitly [string]-cast property. Without the
	// cast, a DateTime property compared against a non-date string throws, because
	// PowerShell tries to convert the right-hand side to the left-hand type.
	whereOpCastString
	// whereOpNumeric compares the property coerced with -as [double], guarded
	// against $null. The guard is required, not cosmetic: bare "$null -lt 1024" is
	// TRUE in PowerShell, so without it every non-numeric value would satisfy
	// lt/le.
	whereOpNumeric
)

// whereOps is the closed set of supported operators, mapping each to the
// PowerShell token emitted for it and how its comparison is built.
//
// It is closed for two reasons: an unrecognized operator becomes a configure-time
// error instead of a filter that silently matches nothing, and no operator text
// from the configuration is ever emitted into the command.
var whereOps = map[string]struct {
	token string
	kind  whereOpKind
}{
	"eq":       {"-eq", whereOpCastString},
	"ne":       {"-ne", whereOpCastString},
	"like":     {"-like", whereOpString},
	"notlike":  {"-notlike", whereOpString},
	"match":    {"-match", whereOpString},
	"notmatch": {"-notmatch", whereOpString},
	"gt":       {"-gt", whereOpNumeric},
	"ge":       {"-ge", whereOpNumeric},
	"lt":       {"-lt", whereOpNumeric},
	"le":       {"-le", whereOpNumeric},
}

// whereEntry keeps or drops an output row based on one of its properties. It is
// compiled into a Where-Object stage of the generated pipeline, so comparisons
// use PowerShell's own semantics rather than a Go reimplementation of them.
//
// Because Where-Object runs before Select-Object, a property used only for
// filtering does not need to be projected and never leaves the child process.
//
// Accepts [Property, Op, Value] or {property: ..., op: ..., value: ...}.
type whereEntry struct {
	Property string
	Op       string
	Value    interface{}
}

// UnmarshalYAML implements dual (positional tuple / mapping) parsing.
func (w *whereEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var seq []interface{}
	if err := unmarshal(&seq); err == nil && len(seq) > 0 {
		if len(seq) != 3 {
			return fmt.Errorf("where tuple must be [Property, Op, Value], got %d elements", len(seq))
		}
		w.Property = scalarToString(seq[0])
		w.Op = scalarToString(seq[1])
		w.Value = seq[2]
		return w.finalize()
	}

	var mp struct {
		Property string      `yaml:"property"`
		Op       string      `yaml:"op"`
		Value    interface{} `yaml:"value"`
	}
	if err := unmarshal(&mp); err != nil {
		return err
	}
	w.Property = mp.Property
	w.Op = mp.Op
	w.Value = mp.Value
	return w.finalize()
}

func (w *whereEntry) finalize() error {
	if w.Property == "" {
		return errors.New("where entry is missing a property")
	}
	// The property name is emitted into the command, so it must satisfy the same
	// identifier rule as every other property this check names.
	if err := validateIdentifier("property", w.Property); err != nil {
		return fmt.Errorf("where entry: %w", err)
	}
	w.Op = strings.ToLower(strings.TrimSpace(w.Op))
	op, ok := whereOps[w.Op]
	if !ok {
		return fmt.Errorf("where entry for property %q has unsupported operator %q", w.Property, w.Op)
	}
	// Values must be scalars: a list or map has no meaningful comparison against a
	// single output property, and only scalars have a safe literal encoding.
	switch w.Value.(type) {
	case nil, bool, string, int, int64, float64:
	default:
		return fmt.Errorf("where entry for property %q must compare against a scalar value", w.Property)
	}
	// An ordering comparison against a non-numeric configured value can never
	// match any row, so reject it here rather than silently collecting nothing.
	if op.kind == whereOpNumeric {
		if _, ok := toFloat(w.Value); !ok {
			return fmt.Errorf("where entry for property %q uses %q, which requires a numeric value, got %v", w.Property, w.Op, w.Value)
		}
	}
	return nil
}

// condition renders this entry as one PowerShell boolean expression, parenthesized
// so it can be joined with others.
//
// Security: the property name is a validated identifier, the operator token comes
// from whereOps and never from configuration text, and the value is encoded as a
// literal (single-quoted with quotes doubled, or an unquoted number). Nothing
// configurable reaches an executable position.
func (w *whereEntry) condition() (string, error) {
	if err := validateIdentifier("property", w.Property); err != nil {
		return "", fmt.Errorf("where entry: %w", err)
	}
	op, ok := whereOps[w.Op]
	if !ok {
		return "", fmt.Errorf("where entry for property %q has unsupported operator %q", w.Property, w.Op)
	}

	switch op.kind {
	case whereOpNumeric:
		lit, err := powershellLiteral(w.Value)
		if err != nil {
			return "", fmt.Errorf("where entry for property %q: %w", w.Property, err)
		}
		// -and binds looser than the comparison operators, so the two halves group
		// as intended without inner parentheses.
		cast := fmt.Sprintf("($_.%s -as [double])", w.Property)
		return fmt.Sprintf("(%s -ne $null -and %s %s %s)", cast, cast, op.token, lit), nil
	case whereOpCastString:
		return fmt.Sprintf("([string]$_.%s %s %s)", w.Property, op.token, singleQuote(scalarToString(w.Value))), nil
	default: // whereOpString
		return fmt.Sprintf("($_.%s %s %s)", w.Property, op.token, singleQuote(scalarToString(w.Value))), nil
	}
}

// whereClause joins every entry with -and into a single Where-Object scriptblock
// body, or returns the empty string when no filtering is configured.
func whereClause(entries []whereEntry) (string, error) {
	if len(entries) == 0 {
		return "", nil
	}
	conds := make([]string, 0, len(entries))
	for i := range entries {
		c, err := entries[i].condition()
		if err != nil {
			return "", err
		}
		conds = append(conds, c)
	}
	return strings.Join(conds, " -and "), nil
}
