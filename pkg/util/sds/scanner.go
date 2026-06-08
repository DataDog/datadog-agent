// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

//nolint:revive
package sds

import (
	"encoding/json"
	"fmt"
	"sync"

	sds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
)

// Match is a single sensitive-data match. It re-exports the upstream
// dd_sds.RuleMatch so callers work directly with the library structure.
type Match = sds.RuleMatch

// RuleDefinition describes a single scanning rule pushed at runtime (e.g. via
// Remote Configuration). Only ID and Regex are required to build the scanner;
// the remaining fields are kept so callers can attribute matches and forward
// rule metadata.
type RuleDefinition struct {
	ID       string
	Name     string
	Regex    string
	Priority string
	Tags     []string
	Labels   []string
}

// scannerState bundles a live scanner with the ordered rule ids it was built
// from, so RuleID can resolve a match index back to its rule id.
type scannerState struct {
	scanner *sds.Scanner
	ruleIDs []string
}

// state holds the process-wide scanner. It starts nil and is lazily built as an
// empty scanner on first use, then replaced wholesale by Reconfigure.
var (
	stateMu sync.RWMutex
	state   *scannerState
)

// getScanner returns the current scanner state, lazily building the built-in
// scanner on first use.
func getScanner() (*scannerState, error) {
	stateMu.RLock()
	s := state
	stateMu.RUnlock()
	if s != nil {
		return s, nil
	}

	stateMu.Lock()
	defer stateMu.Unlock()
	if state != nil {
		return state, nil
	}
	// Build an empty scanner: there are no built-in rules until the scanner is
	// configured via Reconfigure.
	scanner, err := sds.CreateScanner([]sds.RuleConfig{})
	if err != nil {
		return nil, err
	}
	state = &scannerState{scanner: scanner, ruleIDs: nil}
	return state, nil
}

// Reconfigure rebuilds the process-wide scanner from the given rules, replacing
// the built-in (or previously configured) one. Subsequent Scan/ScanMap/RuleID
// calls use the new rule set. The scanner is unique: there is a single,
// process-wide scanner shared by all callers.
func Reconfigure(rules []RuleDefinition) error {
	cfgs := make([]sds.RuleConfig, 0, len(rules))
	ids := make([]string, 0, len(rules))
	fmt.Printf("RECONFIGURE:: %+v\n", rules)
	for _, r := range rules {
		fmt.Printf("RECONFIGURE RULE REGEX:: %+v\n", r.Regex)
		cfgs = append(cfgs, sds.NewMatchingRule(r.ID, r.Regex, sds.ExtraConfig{}))
		ids = append(ids, r.ID)
	}
	scanner, err := sds.CreateScanner(cfgs)
	if err != nil {
		return err
	}
	stateMu.Lock()
	state = &scannerState{scanner: scanner, ruleIDs: ids}
	stateMu.Unlock()
	return nil
}

// RuleID returns the configured rule id for the given match index, or "" when
// the index is out of range.
func RuleID(idx uint32) string {
	stateMu.RLock()
	defer stateMu.RUnlock()
	if state != nil && int(idx) < len(state.ruleIDs) {
		return state.ruleIDs[idx]
	}
	return ""
}

// Scan runs a plain string event through the default SDS scanner and returns
// the matches found (empty when nothing matched).
func Scan(event []byte) ([]Match, error) {
	s, err := getScanner()
	if err != nil {
		return nil, err
	}
	result, err := s.scanner.Scan(event)
	if err != nil {
		return nil, err
	}
	return result.Matches, nil
}

// ScanMap runs a structured event (e.g. a database row) through the default SDS
// scanner using the library's map scanning (ScanEventsMap) and returns the
// matches found. The map values may be mutated (redacted) in place.
func ScanMap(event map[string]interface{}) ([]Match, error) {
	s, err := getScanner()
	if err != nil {
		return nil, err
	}
	result, err := s.scanner.ScanEventsMap(event)
	if err != nil {
		return nil, err
	}

	j, err := json.Marshal(result.Matches)
	fmt.Printf("RESULT MATCHES:: %s --- %+v\n", j, err)
	return result.Matches, nil
}
