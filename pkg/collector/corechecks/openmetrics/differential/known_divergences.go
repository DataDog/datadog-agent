// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"regexp"
	"strings"
)

// KnownDivergence is a recognized, already-documented divergence between the
// Go and Python OpenMetrics implementations. The fuzz target uses this list
// to skip inputs whose only finding is a known divergence — otherwise the
// coverage-guided engine spends all its budget repeatedly minimizing inputs
// for divergences we already understand.
//
// `TestOpenMetricsMutation` and `TestOpenMetricsAdversarial` deliberately
// DO NOT consult this list: they exist to document divergences, so a
// regression in this list (e.g. a fix landing on the Go side) should make
// those tests start passing again, signalling that the entry can be removed.
type KnownDivergence struct {
	Name        string         // short identifier for logs
	Description string         // why this is here
	GoErrRe     *regexp.Regexp // match against iterationOutcome.GoErr (nil means "don't care")
	PyErrSub    string         // substring match against iterationOutcome.PyErr ("" means "don't care")
}

// Matches reports whether `out` looks like this known divergence. All non-nil
// fields must match; absent fields are wildcards. (Empty PyErrSub matches
// against any PyErr including the empty string, which is the right behavior
// when GoErrRe is what's distinguishing the divergence.)
func (k KnownDivergence) Matches(out iterationOutcome) bool {
	if k.GoErrRe != nil {
		if out.GoErr == nil || !k.GoErrRe.MatchString(out.GoErr.Error()) {
			return false
		}
	}
	if k.PyErrSub != "" {
		if !strings.Contains(out.PyErr, k.PyErrSub) {
			return false
		}
	}
	return true
}

// knownDivergences enumerates the divergences documented in README.md's
// "Findings so far" section. Adding an entry here silences the fuzz target
// for that class so it can keep finding NEW classes.
var knownDivergences = []KnownDivergence{
	{
		Name:        "openmetrics_type_keyword",
		Description: "Go rejects scrape on stateset/gaugehistogram/info TYPE (Python degrades)",
		GoErrRe:     regexp.MustCompile(`invalid metric type "(stateset|gaugehistogram|info|unknown)"`),
	},
	{
		Name:        "openmetrics_exemplar",
		Description: "Go rejects scrape on OpenMetrics exemplar trailers (Python skips them)",
		GoErrRe:     regexp.MustCompile(`expected timestamp or new record, got "#"`),
	},
	{
		Name:        "float64_overflow",
		Description: "Go rejects scrape on value out of float64 range; Python clamps to ±Inf",
		GoErrRe:     regexp.MustCompile(`strconv\.ParseFloat:.*value out of range`),
	},
	{
		Name:        "py_reserved_label_prefix",
		Description: "Python rejects sample with __-prefixed label (strict spec); Go accepts",
		PyErrSub:    "Reserved label metric name: __",
	},
	{
		Name:        "py_duplicate_label_name",
		Description: "Python rejects sample with duplicate label name; Go accepts (last-wins)",
		PyErrSub:    "duplicate label name",
	},
	{
		Name:        "py_invalid_labels_value",
		Description: "Python catch-all for malformed label sets (e.g. unescaped newline)",
		PyErrSub:    "Invalid labels:",
	},
	{
		Name:        "py_float_conversion",
		Description: "Python rejects scrape on non-numeric quantile / value string; Go accepts or rejects with a different error",
		PyErrSub:    "could not convert string to float",
	},
	{
		Name:        "go_parse_error_nul_byte",
		Description: "Go rejects scrape on NUL byte in metric name; Python skips the line",
		GoErrRe:     regexp.MustCompile(`expected (value|timestamp|new record).*got "\\x00"`),
	},
	{
		Name:        "go_parse_error_unsupported_char",
		Description: "Go rejects scrape on unsupported character (e.g. hex float syntax); Python may also reject",
		GoErrRe:     regexp.MustCompile(`unsupported character in float`),
	},
}

// IsKnownDivergence reports whether the outcome matches any documented
// divergence class. Used by the fuzz target to suppress noise and focus the
// engine's budget on NEW divergence classes.
func IsKnownDivergence(out iterationOutcome) (bool, string) {
	for _, kd := range knownDivergences {
		if kd.Matches(out) {
			return true, kd.Name
		}
	}
	return false, ""
}
