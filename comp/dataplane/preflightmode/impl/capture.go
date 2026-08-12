// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"sync"

	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Normalized log levels. ADP emits its level as an uppercase string.
const (
	levelError = "ERROR"
	levelWarn  = "WARN"
)

// targetUnstructured is the pseudo-target used for output that did not come through ADP's
// logger, so such lines are still groupable.
const targetUnstructured = "<unstructured>"

// Bounds on what one record may contribute and on how much is retained overall.
//
// ADP is not chatty, but a process failing to start can loop on the same error, and an
// unbounded buffer in the Agent is not an acceptable outcome of a pre-flight. Deduplication
// does most of the work here — a loop on one error costs a single record — and these caps bound
// the pathological case. Every retained field is bounded, so worst-case retention is
// arithmetic rather than a running total: maxRecords * (maxSignatureLen + maxTargetLen), plus
// at most maxLineBytes of unparsed input.
const (
	// maxLineBytes bounds one physical line. ADP renders whole anyhow chains into a message,
	// so records run long.
	maxLineBytes = 1 << 20 // 1 MiB

	// maxSignatureLen bounds a signature, and maxTargetLen a target. Both come from ADP's
	// output, so neither can be trusted to be short.
	maxSignatureLen = 400
	maxTargetLen    = 128

	// maxRecords bounds the retained set.
	maxRecords = 500
	// maxContextRecords bounds the share of the retained set that is neither an error nor a
	// warning, so a chatty INFO stream cannot crowd out the findings the pre-flight exists to
	// surface.
	maxContextRecords = 200
)

// logRecord is one log event from ADP, in the form the capture retains.
//
// The JSON tags describe ADP's NDJSON format and a line is unmarshalled straight into the
// retained form: parseRecord rewrites Level into a normalized level and the message into a
// bounded signature in place, so there is no separate wire type and no copy between the two.
// A real record (agent-data-plane 1.4.0, log_format_json: true):
//
//	{"timestamp":"2026-07-27T17:57:51.708503Z","level":"INFO",
//	 "message":"DogStatsD listener started.","listen_addr":"unixgram:///run/dsd.socket",
//	 "target":"saluki_components::sources::dogstatsd","line_number":1290}
//
// message may be multi-line — ADP renders an anyhow error chain into it — but the newlines
// are escaped inside the JSON string, so a record is always exactly one physical line. That
// is why preflight mode forces JSON: no heuristics are needed to find record boundaries.
//
// Every field is bounded, and the struct is comparable, which is what lets the capture
// deduplicate on a plain map key.
type logRecord struct {
	// Level is normalized: levelError or levelWarn for anything meaning error or warning, and
	// the record's own uppercased level otherwise.
	Level string `json:"level"`
	// Target is the Rust module path, entirely code-determined, so it is safe to keep and
	// useful for grouping.
	Target string `json:"target"`
	// Signature is the message collapsed into a stable, bounded form. It carries the wire
	// field's json tag because it is unmarshalled from it before being rewritten.
	Signature string `json:"message"`
}

// notable reports whether the record is one preflight mode reports on, as opposed to context
// for one that is.
func (r logRecord) notable() bool {
	return r.Level == levelError || r.Level == levelWarn
}

// capture consumes ADP's output as it is produced: it splits the stream into whole lines,
// parses each one as an NDJSON log record, and retains a bounded, deduplicated set of records
// for the post-run scan. Readers iterate the parsed records directly; nothing re-parses.
//
// It is used as cmd.Stdout and cmd.Stderr rather than reading from StdoutPipe, because
// os/exec closes a pipe when Wait sees the command exit — so reading from a pipe races with
// Wait, while assigning a writer makes os/exec own the copy and guarantees Wait returns only
// once all output has been written here.
//
// Lines are parsed whole or dropped whole, never truncated. A truncated JSON record would be
// indistinguishable from output that bypassed ADP's logger, which the scan reports as an
// error — so truncating would manufacture failures out of ordinary long records.
type capture struct {
	log logcomp.Component

	mu      sync.Mutex
	pending []byte
	// discarding is set once the current line has outgrown the buffer, so the rest of it is
	// skipped up to the next newline instead of being parsed as if it were a whole record.
	discarding bool

	records []logRecord
	seen    map[logRecord]struct{}
	// contexts counts retained records that are not notable, against maxContextRecords.
	contexts int
	dropped  int
}

func newCapture(log logcomp.Component) *capture {
	return &capture{log: log, seen: make(map[logRecord]struct{})}
}

// Write implements io.Writer. It always reports the full length: a short write would make
// os/exec's io.Copy treat the stream as failed.
func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := len(p)
	for len(p) > 0 {
		line, rest, terminated := bytes.Cut(p, []byte{'\n'})
		p = rest

		if c.discarding {
			// Still skipping an over-long line; it ends at the next newline, if this chunk
			// even contains one.
			c.discarding = !terminated
			continue
		}

		c.pending = append(c.pending, line...)
		switch {
		case len(c.pending) > maxLineBytes:
			// Dropped whole whether or not the end of the line has arrived yet, so how
			// os/exec happened to chunk the stream cannot change the outcome. If it has not
			// arrived, the rest of the line is skipped rather than parsed as its own record.
			c.pending = c.pending[:0]
			c.dropped++
			c.discarding = !terminated
		case terminated:
			c.commitLocked()
		}
	}
	return total, nil
}

// commitLocked parses the pending bytes as one record and empties the buffer.
func (c *capture) commitLocked() {
	line := string(bytes.TrimRight(c.pending, "\r"))
	c.pending = c.pending[:0]
	if line == "" {
		return
	}

	// Mirrored at debug only: the raw stream can carry operator-controlled text, so it stays
	// local and reachable in a flare via the Agent log, never shipped. This is the only place
	// a line survives verbatim, since what is retained is the parsed record.
	c.log.Debugf("ADP-PREFLIGHT-MODE: %s", line)

	rec, ok := parseRecord(line)
	if !ok {
		return
	}

	// Deduplicated in first-seen order, which is what keeps a process looping on one error
	// from filling the buffer. A duplicate loses no information, so it is not a drop.
	if _, dup := c.seen[rec]; dup {
		return
	}
	if len(c.records) >= maxRecords || (!rec.notable() && c.contexts >= maxContextRecords) {
		c.dropped++
		return
	}
	c.seen[rec] = struct{}{}
	c.records = append(c.records, rec)
	if !rec.notable() {
		c.contexts++
	}
}

// snapshot returns the records parsed so far and how many were dropped. It is read-only: a
// partial line is deliberately not parsed, since half a record would look like output that
// bypassed ADP's logger.
func (c *capture) snapshot() ([]logRecord, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.records), c.dropped
}

// finish parses a trailing line that was never newline-terminated and returns the complete
// capture. Call it once, after cmd.Wait has returned.
func (c *capture) finish() ([]logRecord, int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pending) > 0 && !c.discarding {
		c.commitLocked()
	}
	c.pending = c.pending[:0]
	return slices.Clone(c.records), c.dropped
}

// parseRecord parses one line of ADP output and reports whether it is a log event.
//
// Preflight mode forces JSON, so anything that does not parse bypassed ADP's logger entirely — a
// Rust panic writes straight to stderr, as do linker failures and allocator aborts. Those are
// serious, so unparseable output is reported as an error rather than ignored.
//
// The exception is a line that starts like a JSON object but does not parse: that is a partial
// record — a line dropped for length, or the trailing fragment of a process killed mid-write —
// not output that bypassed the logger, so blaming it would be a false positive. It is skipped;
// the loss is already reported via findingOutputDropped.
func parseRecord(line string) (logRecord, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return logRecord{}, false
	}

	var rec logRecord
	if err := json.Unmarshal([]byte(trimmed), &rec); err != nil {
		if strings.HasPrefix(trimmed, "{") {
			return logRecord{}, false
		}
		return logRecord{Level: levelError, Target: targetUnstructured, Signature: signature(trimmed)}, true
	}

	level, ok := normalizeLevel(rec.Level)
	if !ok {
		return logRecord{}, false
	}
	rec.Level = level

	rec.Signature = signature(rec.Signature)
	if rec.Signature == "" {
		rec.Signature = "(no message)"
	}
	rec.Target = truncate(rec.Target, maxTargetLen)
	if rec.Target == "" {
		rec.Target = "<unknown>"
	}
	return rec, true
}

// normalizeLevel maps ADP's level string onto the levels the scan reasons about.
//
// Levels below a warning are kept under their own name: they are not findings, but they are
// the trail leading up to one, and discarding them would leave a failure with no context. The
// set is closed because ADP logs through the tracing crate, which has exactly these five
// levels — anything else did not come from ADP's logger, so it is not a log event.
func normalizeLevel(level string) (string, bool) {
	switch normalized := strings.ToUpper(strings.TrimSpace(level)); normalized {
	case "ERROR", "FATAL", "CRITICAL":
		return levelError, true
	case "WARN", "WARNING":
		return levelWarn, true
	case "INFO", "DEBUG", "TRACE":
		return normalized, true
	default:
		return "", false
	}
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

// signature collapses a log message into a stable, bounded form. Embedded newlines are folded
// to " | " so a rendered error chain stays on one line without losing the chain.
func signature(message string) string {
	sig := strings.ReplaceAll(strings.TrimSpace(message), "\n", " | ")
	for _, s := range scrubbers {
		sig = s.re.ReplaceAllString(sig, s.with)
	}
	return truncate(strings.Join(strings.Fields(sig), " "), maxSignatureLen)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

// expectedWarnings are warnings preflight mode provokes by construction, matched on target plus
// a message substring. Without this, findingWarningsInLog would fire on every single run.
var expectedWarnings = []struct{ target, contains string }{
	{target: "agent_data_plane::internal::env", contains: "standalone mode"},
}

// hasErrors reports whether any record was an error.
func hasErrors(records []logRecord) bool {
	for _, r := range records {
		if r.Level == levelError {
			return true
		}
	}
	return false
}

// hasUnexpectedWarnings reports whether any record was a warning preflight mode did not provoke.
//
// Warnings matter because ADP reports some hard blockers at WARN — a rejected API key among
// them — so treating them as noise would miss what the pre-flight exists to catch.
func hasUnexpectedWarnings(records []logRecord) bool {
	for _, r := range records {
		if r.Level == levelWarn && !isExpectedWarning(r) {
			return true
		}
	}
	return false
}

func isExpectedWarning(r logRecord) bool {
	for _, e := range expectedWarnings {
		if r.Target == e.target && strings.Contains(r.Signature, e.contains) {
			return true
		}
	}
	return false
}
