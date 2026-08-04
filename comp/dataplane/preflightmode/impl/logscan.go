// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"encoding/json"
	"regexp"
	"strings"
)

// finding is a bounded enum describing one thing that went wrong during a preflight mode run.
// Values are shipped as a telemetry label, so this set must stay small and must never
// contain anything derived from ADP's output.
type finding string

const (
	findingSpawnFailed   finding = "spawn_failed"    // ADP could not be started
	findingProbeFailed   finding = "probe_failed"    // the probe metric never made it into ADP
	findingExitedEarly   finding = "exited_early"    // ADP exited before we asked it to
	findingStopTimeout   finding = "stop_timeout"    // ADP had to be killed
	findingErrorsInLog   finding = "errors_in_log"   // ADP logged an error
	findingWarningsInLog finding = "warnings_in_log" // ADP logged a warning we did not cause
	findingOutputDropped finding = "output_dropped"  // the capture buffer overflowed
	findingInterrupted   finding = "interrupted"     // the Agent shut down mid-run
)

// resultClean is the result label used when a run produced no findings at all.
const resultClean = "clean"

// allFindings exists so a test can assert the set has not grown without the agent telemetry
// profile being updated to match.
var allFindings = []finding{
	findingSpawnFailed,
	findingProbeFailed,
	findingExitedEarly,
	findingStopTimeout,
	findingErrorsInLog,
	findingWarningsInLog,
	findingOutputDropped,
	findingInterrupted,
}

// Normalized log levels. ADP emits its level as an uppercase string.
const (
	levelError = "ERROR"
	levelWarn  = "WARN"
)

// targetUnstructured is the pseudo-target used for output that did not come through ADP's
// logger, so such lines are still groupable.
const targetUnstructured = "<unstructured>"

// expectedWarnings are warnings preflight mode provokes by construction, matched on target plus
// a message substring. Without this, findingWarningsInLog would fire on every single run.
var expectedWarnings = []struct{ target, contains string }{
	{target: "agent_data_plane::internal::env", contains: "standalone mode"},
}

// adpLogRecord is the subset of ADP's JSON log format that matters here. Real example
// (agent-data-plane 1.4.0, log_format_json: true):
//
//	{"timestamp":"2026-07-27T17:57:51.708503Z","level":"INFO",
//	 "message":"DogStatsD listener started.","listen_addr":"unixgram:///run/dsd.socket",
//	 "target":"saluki_components::sources::dogstatsd","line_number":1290}
//
// message may be multi-line — ADP renders an anyhow error chain into it — but the newlines
// are escaped inside the JSON string, so a record is always exactly one physical line. That
// is why preflight mode forces JSON: no heuristics are needed to find record boundaries.
type adpLogRecord struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Target  string `json:"target"`
}

// scrubbers collapse a message into a stable signature so the same failure groups across
// hosts. Applied in order.
var scrubbers = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`), "<ts>"},
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`), "<uuid>"},
	{regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`), "<addr>"},
	{regexp.MustCompile(`(?:[A-Za-z]:\\|/)[^\s"'\\]*(?:[\\/][^\s"']*)+`), "<path>"},
	{regexp.MustCompile(`\b\d+\b`), "<n>"},
}

// maxSignatureLen bounds a signature. ADP renders whole error chains into one message, so
// these run long.
const maxSignatureLen = 400

// scannedLine is one classified ADP log record.
type scannedLine struct {
	Level string
	// Target is the Rust module path, entirely code-determined, so it is safe to keep and
	// useful for grouping.
	Target    string
	Signature string
}

// scanOutput classifies ADP's captured output, returning error and warning records
// deduplicated by (level, target, signature) in first-seen order.
//
// Warnings are collected because ADP reports some hard blockers at WARN — a rejected API key
// among them — so treating them as noise would miss what the pre-flight exists to catch.
func scanOutput(lines []string) []scannedLine {
	var out []scannedLine
	seen := make(map[string]struct{}, len(lines))

	for _, line := range lines {
		parsed, ok := classifyLine(line)
		if !ok {
			continue
		}
		key := parsed.Level + "\x00" + parsed.Target + "\x00" + parsed.Signature
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, parsed)
	}
	return out
}

// hasErrors reports whether any record was an error.
func hasErrors(lines []scannedLine) bool {
	for _, l := range lines {
		if l.Level == levelError {
			return true
		}
	}
	return false
}

// hasUnexpectedWarnings reports whether any record was a warning preflight mode did not provoke.
func hasUnexpectedWarnings(lines []scannedLine) bool {
	for _, l := range lines {
		if l.Level == levelWarn && !isExpectedWarning(l) {
			return true
		}
	}
	return false
}

func isExpectedWarning(l scannedLine) bool {
	for _, e := range expectedWarnings {
		if l.Target == e.target && strings.Contains(l.Signature, e.contains) {
			return true
		}
	}
	return false
}

// classifyLine parses one line of ADP output and reports whether it is worth keeping.
//
// Preflight mode forces JSON, so anything that does not parse bypassed ADP's logger entirely — a
// Rust panic writes straight to stderr, as do linker failures and allocator aborts. Those are
// serious, so unparseable output is reported as an error rather than ignored.
//
// The exception is a line that starts like a JSON object but does not parse: that is a record
// the capture buffer dropped, not output that bypassed the logger, so blaming it would be a
// false positive. It is skipped; the loss is already reported via findingOutputDropped.
func classifyLine(line string) (scannedLine, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return scannedLine{}, false
	}

	var rec adpLogRecord
	if err := json.Unmarshal([]byte(trimmed), &rec); err != nil {
		if strings.HasPrefix(trimmed, "{") {
			return scannedLine{}, false
		}
		return scannedLine{Level: levelError, Target: targetUnstructured, Signature: signature(trimmed)}, true
	}

	level, ok := normalizeLevel(rec.Level)
	if !ok {
		return scannedLine{}, false
	}

	sig := signature(rec.Message)
	if sig == "" {
		sig = "(no message)"
	}
	target := rec.Target
	if target == "" {
		target = "<unknown>"
	}
	return scannedLine{Level: level, Target: target, Signature: sig}, true
}

// normalizeLevel maps ADP's level string onto the two levels worth reporting.
func normalizeLevel(level string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "ERROR", "FATAL", "CRITICAL":
		return levelError, true
	case "WARN", "WARNING":
		return levelWarn, true
	default:
		return "", false
	}
}

// signature collapses a log message into a stable, bounded form. Embedded newlines are folded
// to " | " so a rendered error chain stays on one line without losing the chain.
func signature(message string) string {
	sig := strings.ReplaceAll(strings.TrimSpace(message), "\n", " | ")
	for _, s := range scrubbers {
		sig = s.re.ReplaceAllString(sig, s.with)
	}
	sig = strings.Join(strings.Fields(sig), " ")
	if len(sig) > maxSignatureLen {
		sig = sig[:maxSignatureLen]
	}
	return sig
}
