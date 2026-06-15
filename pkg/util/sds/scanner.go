// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

package sds

import (
	ddsds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
)

// scanner is the cgo-backed Scanner. It owns a native SDS scanner handle and
// the ordered rule ids so a match's rule index can be resolved back to its id.
// A scanner is immutable once created: its rule set never changes, so callers
// register a new scanner instead of updating an existing one.
type scanner struct {
	inner   *ddsds.Scanner
	ruleIDs []string
}

// NewScanner builds a scanner from the given rules.
func NewScanner(rules []RuleDefinition) (Scanner, error) {
	cfgs := make([]ddsds.RuleConfig, 0, len(rules))
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		cfgs = append(cfgs, ddsds.NewMatchingRule(r.ID, r.Regex, ddsds.ExtraConfig{}))
		ids = append(ids, r.ID)
	}
	inner, err := ddsds.CreateScanner(cfgs)
	if err != nil {
		return nil, err
	}

	return &scanner{inner: inner, ruleIDs: ids}, nil
}

// Scan runs a plain-string event through the scanner.
func (s *scanner) Scan(event []byte) ([]Match, error) {
	if s.inner == nil {
		return nil, nil
	}
	res, err := s.inner.Scan(event)
	if err != nil {
		return nil, err
	}
	return s.matches(res.Matches), nil
}

// ScanMap runs a structured event (e.g. a database row) through the scanner.
func (s *scanner) ScanMap(event map[string]interface{}) ([]Match, error) {
	if s.inner == nil {
		return nil, nil
	}
	res, err := s.inner.ScanEventsMap(event)
	if err != nil {
		return nil, err
	}
	return s.matches(res.Matches), nil
}

// Close releases the native scanner handle.
func (s *scanner) Close() error {
	if s.inner != nil {
		s.inner.Delete()
		s.inner = nil
	}
	return nil
}

func (s *scanner) matches(raw []ddsds.RuleMatch) []Match {
	out := make([]Match, 0, len(raw))
	for _, m := range raw {
		id := ""
		if int(m.RuleIdx) < len(s.ruleIDs) {
			id = s.ruleIDs[m.RuleIdx]
		}
		out = append(out, Match{RuleID: id, Path: m.Path})
	}
	return out
}
