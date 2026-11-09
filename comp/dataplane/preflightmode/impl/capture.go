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

// sourceUnknown is the source location reported for a record that does not carry a usable one.
// Output that bypassed ADP's logger has no location at all, and a location that fails
// validation is treated the same way rather than reported as though it were real.
const sourceUnknown = "<unknown>"

// Bounds on what one record may contribute and on how much is retained overall.
//
// ADP is not chatty, but a process failing to start can loop on the same error, and an
// unbounded buffer in the Agent is not an acceptable outcome of a pre-flight. Deduplication
// does most of the work here — a loop on one error costs a single record — and these caps bound
// the pathological case. Every retained field is bounded, so worst-case retention is
// arithmetic rather than a running total: maxRecords * (maxSignatureLen + maxTargetLen +
// maxSourceFileLen + maxSourceLineLen), plus at most maxLineBytes of unparsed input. A field
// that is rejected rather than bounded holds a sentinel instead, which is a constant.
const (
	// maxLineBytes bounds one physical line. ADP renders whole anyhow chains into a message,
	// so records run long.
	maxLineBytes = 1 << 20 // 1 MiB

	// maxSignatureLen bounds a signature, and maxTargetLen a target. Both come from ADP's
	// output, so neither can be trusted to be short.
	maxSignatureLen = 400
	maxTargetLen    = 128

	// maxSourceFileLen bounds a source file path and maxSourceLineLen a line number. Both come
	// from ADP's output too, and both are rejected rather than truncated when they overrun: a
	// truncated path or line number still looks like a real location, and these are reported as
	// telemetry tags where a plausible-looking wrong value is worse than no value. ADP's own
	// paths are relative to its repository root, so the longest real one is well under this.
	maxSourceFileLen = 256
	maxSourceLineLen = 8

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
//	 "target":"saluki_components::sources::dogstatsd",
//	 "filename":"lib/saluki-components/src/sources/dogstatsd/mod.rs","line_number":1290}
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
	// SourceFile and SourceLine are where in ADP's own source the record was logged from.
	// Like Target they are code-determined — ADP's logger fills them in from file!() and
	// line!() — which is what makes them safe to report, and they are what points a finding at
	// the log site that produced it. Both are sourceUnknown when the record carries no usable
	// value, so a location is never half-known.
	SourceFile string     `json:"filename"`
	SourceLine sourceLine `json:"line_number"`
}

// sourceLine is a record's line number, retained as text because it is only ever reported as a
// tag value and never counted with.
//
// It has an unmarshaller of its own purely so that the field arriving as anything other than a
// JSON number cannot fail the unmarshal and take the whole record down with it. The location is
// a tag; losing an error record because ADP started quoting its line numbers would be a poor
// trade. Whatever arrives is validated by normalizeSourceLine before it is retained.
type sourceLine string

// UnmarshalJSON accepts any JSON value, quoted or not, and never fails.
func (l *sourceLine) UnmarshalJSON(data []byte) error {
	*l = sourceLine(bytes.Trim(data, `"`))
	return nil
}

// sourceLocation is one log site in ADP's source. Both fields are already normalized and
// bounded by the time a location is built from a record, because this is what reaches Datadog
// as a tag.
type sourceLocation struct {
	file string
	line string
}

// location returns the log site the record came from.
func (r logRecord) location() sourceLocation {
	return sourceLocation{file: r.SourceFile, line: string(r.SourceLine)}
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
		return logRecord{
			Level:      levelError,
			Target:     targetUnstructured,
			Signature:  signature(trimmed),
			SourceFile: sourceUnknown,
			SourceLine: sourceUnknown,
		}, true
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
	rec.SourceFile = normalizeSourceFile(rec.SourceFile)
	rec.SourceLine = sourceLine(normalizeSourceLine(string(rec.SourceLine)))
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

// sourceFilePattern is what a filename has to match to be reported, once its separators have
// been folded to forward slashes.
//
// ADP fills the field in from file!(), so it is a compile-time constant of ADP's own build and
// carries nothing operator-controlled. The character set is enforced regardless, because this is
// the only label value on the finding metric that is read out of ADP's output rather than chosen
// from a constant here: a value carrying a comma, a colon or whitespace would not survive being
// turned into a tag intact, and anything that does not look like a source path is not one. That
// colon still rejects an absolute Windows path on its drive letter, which is deliberate and
// costs nothing we have: ADP's own paths are workspace-relative, so a drive letter only shows up
// on a log site inside a dependency crate, where the path describes the build machine's cargo
// registry rather than anything a reader can navigate to.
var sourceFilePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// normalizeSourceFile returns the file a record was logged from, or sourceUnknown if it did not
// carry one that can be reported.
//
// Backslashes are folded to forward slashes first, because file!() bakes in the path form of the
// machine ADP was built on: a Windows build logs `bin\agent-data-plane\src\main.rs` where a Linux
// build of the same source logs `bin/agent-data-plane/src/main.rs`. Folded rather than admitted
// into the character set above, so that one log site is one tag value across a mixed fleet
// instead of two. The replacement is explicit rather than filepath.ToSlash because this is a
// string out of ADP's output and not a path on this machine: ToSlash compiles to a no-op
// everywhere but Windows, which would both leave the fold to the Agent's build platform and
// leave this case unexercised on a Linux test runner.
func normalizeSourceFile(file string) string {
	file = strings.ReplaceAll(strings.TrimSpace(file), `\`, "/")
	if file == "" || len(file) > maxSourceFileLen || !sourceFilePattern.MatchString(file) {
		return sourceUnknown
	}
	return file
}

// normalizeSourceLine returns the line a record was logged from, or sourceUnknown if it did not
// carry one that can be reported. Only digits are accepted: sourceLine takes whatever the field
// held, so this is where a value that is not a line number is rejected.
func normalizeSourceLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || len(line) > maxSourceLineLen {
		return sourceUnknown
	}
	for _, c := range []byte(line) {
		if c < '0' || c > '9' {
			return sourceUnknown
		}
	}
	return line
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

// isError reports whether the record is an error.
func isError(r logRecord) bool {
	return r.Level == levelError
}

// isUnexpectedWarning reports whether the record is a warning preflight mode did not provoke.
func isUnexpectedWarning(r logRecord) bool {
	return r.Level == levelWarn && !isExpectedWarning(r)
}

// hasErrors reports whether any record was an error.
func hasErrors(records []logRecord) bool {
	return slices.ContainsFunc(records, isError)
}

// hasUnexpectedWarnings reports whether any record was a warning preflight mode did not provoke.
//
// Warnings matter because ADP reports some hard blockers at WARN — a rejected API key among
// them — so treating them as noise would miss what the pre-flight exists to catch.
func hasUnexpectedWarnings(records []logRecord) bool {
	return slices.ContainsFunc(records, isUnexpectedWarning)
}

func isExpectedWarning(r logRecord) bool {
	for _, e := range expectedWarnings {
		if r.Target == e.target && strings.Contains(r.Signature, e.contains) {
			return true
		}
	}
	return false
}

// locationsOf returns the distinct log sites of the records matching keep, in first-seen order.
//
// Distinct rather than one per record because the same site logging twice says nothing more than
// it logging once — the capture already deduplicates identical records, and two records from one
// site differ only in the detail their message carries, which is not shipped. First-seen order so
// that a caller keeping a prefix of the result keeps the earliest sites, which are the most
// explanatory ones.
func locationsOf(records []logRecord, keep func(logRecord) bool) []sourceLocation {
	var locations []sourceLocation
	seen := make(map[sourceLocation]struct{})
	for _, r := range records {
		if !keep(r) {
			continue
		}
		loc := r.location()
		if _, dup := seen[loc]; dup {
			continue
		}
		seen[loc] = struct{}{}
		locations = append(locations, loc)
	}
	return locations
}
