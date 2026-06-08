// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sds

// Package sds wraps the Datadog Sensitive Data Scanner shared library and
// exposes Scan/ScanMap helpers backed by a process-wide default scanner. It is
// only functional when the Agent is built with the `sds` build tag and the
// shared library is available; otherwise the helpers are no-ops returning no
// matches.
//
// The Match type is defined per build tag: with `sds` it is a re-export
// (alias) of the upstream dd_sds.RuleMatch; without it, the stand-in below
// mirrors the fields consumers rely on (the cgo upstream package cannot be
// imported in the no-sds build).
//
//nolint:revive
package sds

// Match mirrors the fields of the upstream dd_sds.RuleMatch that consumers use.
// The no-sds build never populates it (Scan always returns no matches).
type Match struct {
	RuleIdx           uint32
	Path              string
	StartIndex        uint32
	EndIndexExclusive uint32
	MatchStatus       string
}

// RuleDefinition describes a single scanning rule pushed at runtime (e.g. via
// Remote Configuration). The no-sds build ignores it.
type RuleDefinition struct {
	ID       string
	Name     string
	Regex    string
	Priority string
	Tags     []string
	Labels   []string
}

// Reconfigure is a no-op when the Agent is built without the `sds` build tag:
// there is no scanner to reconfigure.
func Reconfigure(_ []RuleDefinition) error {
	return nil
}

// RuleID returns the configured rule id for the given match index. The no-sds
// build has no rules, so it always returns "".
func RuleID(_ uint32) string {
	return ""
}

// Scan is a no-op when the Agent is built without the `sds` build tag: it
// returns no matches.
func Scan(_ []byte) ([]Match, error) {
	return nil, nil
}

// ScanMap is a no-op when the Agent is built without the `sds` build tag: it
// returns no matches and leaves the event untouched.
func ScanMap(_ map[string]interface{}) ([]Match, error) {
	return nil, nil
}
