// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
)

// Symbols used to render filtered Pulumi resource events.
const (
	progressSymbolInProgress = "⏳"
	progressSymbolDone       = "✓"
	progressSymbolFailed     = "✗"
	progressSymbolUnknown    = "?"
)

// progressLineRe matches the resource progress lines emitted by the Pulumi
// engine, such as:
//
//	" +  aws:eks:Cluster myeks creating..."
//	" ~  aws:ec2:SecurityGroup myeks-sg updated (12s)"
//	" -  aws:s3:Bucket my-bucket deleted (3s)"
//
// Capture groups: 1: resource type, 2: resource name, 3: remainder.
var progressLineRe = regexp.MustCompile(`^ *[+~>\-!] +([^ ]+) +([^ ]+)(?: +(.*))?$`)

// progressSymbols maps the verb of a resource event to its display symbol.
var progressSymbols = map[string]string{
	"creating": progressSymbolInProgress,
	"updating": progressSymbolInProgress,
	"deleting": progressSymbolInProgress,
	"reading":  progressSymbolInProgress,
	"created":  progressSymbolDone,
	"updated":  progressSymbolDone,
	"deleted":  progressSymbolDone,
	"read":     progressSymbolDone,
}

// progressFilter is an io.Writer that sits between the Pulumi progress stream
// and the final logger. It parses the raw engine output line by line and only
// forwards clean, formatted resource events, suppressing engine internals,
// summary noise and empty lines.
type progressFilter struct {
	dest io.Writer
	mu   sync.Mutex
	buf  string
}

// NewProgressFilter returns an io.Writer that filters raw Pulumi progress
// output before writing it to w.
func NewProgressFilter(w io.Writer) io.Writer {
	return &progressFilter{dest: w}
}

// Write implements io.Writer. Lines are reassembled from arbitrary chunks
// before being filtered, so partial progress lines are never emitted.
func (f *progressFilter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.buf += string(p)
	for {
		idx := strings.IndexByte(f.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSuffix(f.buf[:idx], "\r")
		f.buf = f.buf[idx+1:]

		if out, show := formatProgressLine(line); show {
			// The destination is terminal-like output; write errors are not
			// fatal for the deployment and are intentionally ignored.
			_, _ = fmt.Fprintln(f.dest, out)
		}
	}

	return len(p), nil
}

// formatProgressLine converts a raw Pulumi engine progress line into a clean,
// formatted line. It returns false when the line should be suppressed.
func formatProgressLine(line string) (string, bool) {
	if strings.TrimSpace(line) == "" {
		return "", false
	}

	if m := progressLineRe.FindStringSubmatch(line); m != nil {
		resType, name, rest := m[1], m[2], m[3]
		// Provider reads and other engine internals are noise, and a resource
		// line without a verb is malformed (e.g. summary " + 23 created").
		if strings.HasPrefix(resType, "pulumi:providers:") || rest == "" {
			return "", false
		}
		return formatResourceEvent(resType, name, rest)
	}

	// Not a resource progress line: only forward lines that look important,
	// such as engine diagnostics, and drop the rest of the noise.
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	for _, keyword := range []string{"error", "warning", "failed", "diagnostic"} {
		if strings.Contains(lower, keyword) {
			return trimmed, true
		}
	}

	return "", false
}

// formatResourceEvent renders a resource progress event as
// "<symbol> <type> <name> <verb...>".
func formatResourceEvent(resType, name, rest string) (string, bool) {
	// Operation failures are reported as "**creating failed** error: <details>".
	if strings.Contains(rest, "failed") {
		if idx := strings.Index(rest, "error:"); idx >= 0 {
			if details := strings.TrimSpace(rest[idx+len("error:"):]); details != "" {
				return fmt.Sprintf("  %s %s %s failed: %s", progressSymbolFailed, resType, name, details), true
			}
		}
		return fmt.Sprintf("  %s %s %s failed", progressSymbolFailed, resType, name), true
	}

	verb := strings.Fields(rest)[0]
	if strings.HasSuffix(verb, "...") {
		// e.g. "creating...": the operation is still running.
		return fmt.Sprintf("  %s %s %s %s", progressSymbolInProgress, resType, name, verb), true
	}

	if symbol, ok := progressSymbols[strings.Trim(verb, "*.")]; ok {
		// e.g. "created (45s)" or "updated (1m10s)": the operation completed.
		return fmt.Sprintf("  %s %s %s %s", symbol, resType, name, rest), true
	}

	// Unrecognized verb on an otherwise valid resource line: show the raw
	// remainder rather than hiding potentially important information.
	return fmt.Sprintf("  %s %s %s %s", progressSymbolUnknown, resType, name, rest), true
}
