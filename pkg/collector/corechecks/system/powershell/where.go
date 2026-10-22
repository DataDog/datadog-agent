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

// normalizeOp canonicalizes a configured operator. Used both here and by schema
// validation, so the two layers cannot disagree about what a caller may write.
func normalizeOp(op string) string {
	return strings.ToLower(strings.TrimSpace(op))
}

// whereOps is the closed set of supported operators, mapped to the Where-Object
// switch each one binds. schema.go duplicates the list; keep the two in step.
var whereOps = map[string]string{
	"eq":       "EQ",
	"ne":       "NE",
	"like":     "Like",
	"notlike":  "NotLike",
	"match":    "Match",
	"notmatch": "NotMatch",
	"gt":       "GT",
	"ge":       "GE",
	"lt":       "LT",
	"le":       "LE",
}

// whereEntry keeps or drops a row on one property, compiled into a Where-Object
// stage so comparisons use PowerShell's semantics. Accepts tuple or mapping form.
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
	// Bound to Where-Object -Property, so restrict it to a plain identifier and
	// reject a malformed name at configure time.
	if err := validateIdentifier("property", w.Property); err != nil {
		return fmt.Errorf("where entry: %w", err)
	}
	w.Op = normalizeOp(w.Op)
	if _, ok := whereOps[w.Op]; !ok {
		return fmt.Errorf("where entry for property %q has unsupported operator %q", w.Property, w.Op)
	}
	// Scalars only: the allowlist checks a value as a string while the payload
	// carries JSON, and those two agree only for scalars.
	switch w.Value.(type) {
	case nil, bool, string, int, int64, float64:
	default:
		return fmt.Errorf("where entry for property %q must compare against a scalar value", w.Property)
	}
	return nil
}

// writeStage appends this entry's declaration to decls and its pipeline stage to
// pipe, for payload index i. Nothing configurable is written: property and value
// are read from $cfg, and only i and the whereOps switch are interpolated.
func (w *whereEntry) writeStage(decls, pipe *strings.Builder, i int) error {
	if err := validateIdentifier("property", w.Property); err != nil {
		return fmt.Errorf("where entry: %w", err)
	}
	sw, ok := whereOps[w.Op]
	if !ok {
		return fmt.Errorf("where entry for property %q has unsupported operator %q", w.Property, w.Op)
	}

	fmt.Fprintf(decls, "$wp%d = @{ Property = $cfg.where[%d].property; Value = $cfg.where[%d].value; %s = $true }\n", i, i, i, sw)
	fmt.Fprintf(pipe, " | & $w @wp%d", i)
	return nil
}
