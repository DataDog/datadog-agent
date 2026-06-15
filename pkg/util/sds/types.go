// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sds wraps the Datadog sensitive-data-scanner (SDS) library as
// scanner instances. The real, cgo-backed scanner is only compiled under the
// `sds` build tag; every other build gets the no-op scanner returned by
// NoOpScanner (and by NewScanner).
package sds

// Match is a single sensitive-data hit: the id of the rule that fired and, for
// structured (map) scans, the path of the value it matched.
type Match struct {
	RuleID string
	Path   string
}

// RuleDefinition describes one scanning rule. Only ID and Regex are required;
// the remaining fields are placeholders for richer rule configuration.
type RuleDefinition struct {
	ID    string
	Name  string
	Regex string
}

// Scanner scans events for sensitive data. Implementations are safe for
// concurrent use; Close releases any native resources held by the scanner.
type Scanner interface {
	Scan(event []byte) ([]Match, error)
	ScanMap(event map[string]interface{}) ([]Match, error)
	Close() error
}

// NoOpScanner returns a Scanner that matches nothing. It is used when SDS is
// turned off by configuration, or when the Agent is built without the `sds`
// build tag, so callers can depend on a Scanner unconditionally.
func NoOpScanner() Scanner { return noopScanner{} }

type noopScanner struct{}

func (noopScanner) Scan([]byte) ([]Match, error)                    { return nil, nil }
func (noopScanner) ScanMap(map[string]interface{}) ([]Match, error) { return nil, nil }
func (noopScanner) Close() error                                    { return nil }
