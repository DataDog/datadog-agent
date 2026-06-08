// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

//nolint:revive
package sds

import (
	"sync"

	sds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
)

// defaultRules is the built-in rule set the scanner uses until it is replaced
// by Reconfigure. It scans for email addresses and IPv4 addresses.
var defaultRules = []RuleDefinition{
	{
		ID:    "PuXiVTCkTHOtj0Yad1ppsw",
		Name:  "Standard Email Address Scanner",
		Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`,
	},
	{
		ID:    "aDA3jUjSSLOezHV2y-Rn_w",
		Name:  "IPv4 Address Scanner",
		Regex: `(?:\d+\.){3}\d+`,
	},
}

// ScannerState bundles the live process-wide scanner with the ordered rule ids
// it was built from, so a match's rule index can be resolved back to its id.
type ScannerState struct {
	scanner *sds.Scanner
	ruleIDs []string
}

// matches converts the upstream rule matches into our Match type, resolving each
// match's rule index back to the configured rule id.
func (s *ScannerState) matches(raw []sds.RuleMatch) []Match {
	out := make([]Match, 0, len(raw))
	for _, m := range raw {
		ruleID := ""
		if int(m.RuleIdx) < len(s.ruleIDs) {
			ruleID = s.ruleIDs[m.RuleIdx]
		}
		out = append(out, Match{RuleID: ruleID, Path: m.Path})
	}
	return out
}

// state holds the process-wide scanner. It starts nil and is lazily built from
// the built-in defaultRules on first use, then replaced wholesale by Reconfigure.
var (
	stateMu sync.RWMutex
	state   *ScannerState
)

// buildState creates a scanner from the given rules, keeping the ordered rule
// ids so matches can be resolved back to their rule id.
func buildState(rules []RuleDefinition) (*ScannerState, error) {
	cfgs := make([]sds.RuleConfig, 0, len(rules))
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		cfgs = append(cfgs, sds.NewMatchingRule(r.ID, r.Regex, sds.ExtraConfig{}))
		ids = append(ids, r.ID)
	}
	scanner, err := sds.CreateScanner(cfgs)
	if err != nil {
		return nil, err
	}
	return &ScannerState{scanner: scanner, ruleIDs: ids}, nil
}

// getScanner returns the current scanner state, lazily building the built-in
// scanner (from defaultRules) on first use.
func getScanner() (*ScannerState, error) {
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
	built, err := buildState(defaultRules)
	if err != nil {
		return nil, err
	}
	state = built
	return state, nil
}

// Reconfigure rebuilds the process-wide scanner from the given rules, replacing
// the built-in (or previously configured) one. Subsequent Scan/ScanMap calls
// use the new rule set. The scanner is unique: there is a single, process-wide
// scanner shared by all callers.
func Reconfigure(rules []RuleDefinition) error {
	built, err := buildState(rules)
	if err != nil {
		return err
	}
	stateMu.Lock()
	state = built
	stateMu.Unlock()
	return nil
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
	return s.matches(result.Matches), nil
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
	return s.matches(result.Matches), nil
}
