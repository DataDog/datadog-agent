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
	// The architectural-difference class: Go aborts the scrape on essentially
	// any parse error, while Python skips the offending line. Each entry
	// below is a specific instance of this single underlying bug. Lumping
	// them under one broad regex would prevent the fuzz from finding *new*
	// classes — keep them individually narrow.
	{
		Name:        "go_parse_error_invalid_utf8",
		Description: "Go rejects scrape on invalid UTF-8 in label value; Python tolerates raw bytes",
		GoErrRe:     regexp.MustCompile(`invalid UTF-8 label value`),
	},
	{
		Name:        "go_parse_error_expected_value",
		Description: "Go rejects scrape on missing/unparseable value (NUL, control char, malformed numeric); Python skips the line",
		GoErrRe:     regexp.MustCompile(`expected (value|name) (after|but)`),
	},
	{
		Name:        "go_parse_error_expected_timestamp_or_record",
		Description: "Go rejects scrape on unexpected token between value and timestamp; Python skips the line",
		GoErrRe:     regexp.MustCompile(`expected timestamp or new record, got "[^#]`),
	},
	{
		Name:        "go_parse_error_unsupported_char",
		Description: "Go rejects scrape on unsupported character (hex float, form feed, etc.); Python may also reject",
		GoErrRe:     regexp.MustCompile(`unsupported character`),
	},
	{
		Name:        "go_parse_error_unterminated_label",
		Description: "Go rejects scrape on truncated label set or unterminated string in label value",
		GoErrRe:     regexp.MustCompile(`(unterminated|unexpected EOF|unterminated label)`),
	},
	{
		Name:        "go_parse_error_invalid_metric_or_label_name",
		Description: "Go rejects scrape on metric/label name starting with digit or containing forbidden chars",
		GoErrRe:     regexp.MustCompile(`(invalid metric name|invalid label name|name must|not a valid)`),
	},
	{
		Name:        "go_parse_error_malformed_help_or_type",
		Description: "Go rejects scrape on malformed/truncated `# HELP` or `# TYPE` meta-line",
		GoErrRe:     regexp.MustCompile(`(malformed|truncated|expected metric name).*\b(HELP|TYPE)\b`),
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
