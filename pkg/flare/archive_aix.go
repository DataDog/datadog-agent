// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package flare

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

const (
	// errptWindow is how far back the collected entries go.
	errptWindow = 7 * 24 * time.Hour
	// errptMaxCollect bounds the output retained from one errpt call, after
	// any record filtering.
	errptMaxCollect = 4 * 1024 * 1024
	// errptMaxErrOutput caps the stderr kept in error messages.
	errptMaxErrOutput = 4 * 1024
)

const truncatedMarker = "\n[output truncated]\n"

// Names our processes appear as in the error log: the binaries (CORE_DUMP
// PROGRAM NAME) and the SRC subsystem names (FAILING MODULE, prefix).
var (
	errptBinaries   = []string{"agent-bin", "trace-agent-bin", "agent-data-plane-bin"}
	errptSubsystems = []string{"datadog-"}
)

// getSvmonData captures the AIX virtual memory segment breakdown (heap,
// stack, text, shared library segments) for the running Agent process, since
// resource limits are enforced per segment and RSS/VSZ alone don't show which
// segment is close to its ulimit.
func getSvmonData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	if fb.IsLocal() {
		return nil
	}

	pid := strconv.Itoa(os.Getpid())
	cmd := exec.CommandContext(ctx, "svmon", "-P", pid, "-O", "unit=KB,segment=on")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = fb.Logf("error running svmon -P %s: %s", pid, err)
	}

	return fb.AddFile("svmon.log", out)
}

// getErrptData collects the AIX error log for the last 7 days — often the
// only in-host evidence when the agent dies without leaving anything in its
// own logs. Also runs in local mode, since errpt needs root and the agent
// process (dd-agent) cannot read it.
func getErrptData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	since := errptSince(time.Now().Add(-errptWindow))

	// A924A5FC CORE_DUMP, 620096CD/8527F6F4 out-of/no paging space.
	deaths := runErrpt(ctx, errptBinaries, "-a", "-j", "A924A5FC", "-s", since)
	if deaths.err != nil {
		_ = fb.Logf("error running errpt for process deaths: %s", deaths.err)
	}
	paging := runErrpt(ctx, nil, "-a", "-j", "620096CD,8527F6F4", "-s", since)
	if paging.err != nil {
		_ = fb.Logf("error running errpt for paging space entries: %s", paging.err)
	}
	src := runErrpt(ctx, errptSubsystems, "-a", "-N", "SRC", "-s", since)
	if src.err != nil {
		_ = fb.Logf("error running errpt for SRC entries: %s", src.err)
	}

	return fb.AddFile("errpt.log", buildErrptLog(deaths, paging, src))
}

// errptSince formats t in the MMDDhhmmYY form expected by errpt's -s flag.
func errptSince(t time.Time) string {
	return t.Format("0102150406")
}

type errptResult struct {
	out []byte
	// hadRecords reports whether the raw output had any record, matching or
	// not.
	hadRecords bool
	err        error
}

// runErrpt runs errpt; when match is not nil, only records containing one of
// the strings are kept. The filtering happens while the output is being
// read, so the collection cap bounds the retained output rather than the
// raw one: an Agent record cannot be crowded out by unrelated noise before
// it. stderr is folded into err so query errors are never mistaken for
// error-log content.
func runErrpt(ctx context.Context, match []string, args ...string) errptResult {
	stdout := newErrptWriter(errptMaxCollect, match)
	stderr := &boundedWriter{max: errptMaxErrOutput}
	cmd := exec.CommandContext(ctx, "errpt", args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr

	var res errptResult
	if err := cmd.Run(); err != nil {
		if stderr.buf.Len() > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(stderr.Bytes())))
		}
		res.err = err
	}
	res.out, res.hadRecords = stdout.result()
	return res
}

// errptWriter is errpt's stdout sink. A record is a block of lines closed by
// a separator line of dashes; blocks are filtered as they are read and only
// matching ones are retained (everything when match is nil), bounded by
// max bytes of kept output.
type errptWriter struct {
	match []string
	max   int

	line  bytes.Buffer // pending partial line
	block bytes.Buffer // current record block
	kept  boundedWriter
	// hadRecords reports whether the raw output contained any record,
	// matching or not.
	hadRecords bool
}

func newErrptWriter(max int, match []string) *errptWriter {
	return &errptWriter{match: match, max: max, kept: boundedWriter{max: max}}
}

// Write drains p entirely; non-matching blocks are discarded.
func (w *errptWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.line.Write(p)
			if w.line.Len() > w.max {
				// Safety valve: flush a line that grows past the cap so a
				// stream without newlines cannot grow the buffer forever.
				w.flushLine(w.line.Bytes())
				w.line.Reset()
			}
			break
		}
		w.line.Write(p[:i+1])
		p = p[i+1:]
		w.flushLine(w.line.Bytes())
		w.line.Reset()
	}
	return n, nil
}

// flushLine adds line to the current block, closing the block when the line
// is a separator (dashes only). Blocks larger than the cap are cut short so
// memory stays bounded.
func (w *errptWriter) flushLine(line []byte) {
	w.block.Write(line)
	if w.block.Len() > w.max {
		w.flushBlock()
		return
	}
	content := bytes.TrimSuffix(line, []byte("\n"))
	if len(content) == 0 || len(bytes.Trim(content, "-")) != 0 {
		return
	}
	w.flushBlock()
}

// flushBlock closes the current record block: it counts as a record when it
// holds more than separator dashes, and is kept when it matches the filter
// (always when there is none).
func (w *errptWriter) flushBlock() {
	defer w.block.Reset()
	if len(bytes.Trim(w.block.Bytes(), "-\n")) == 0 {
		return // separators only
	}
	w.hadRecords = true
	if w.match != nil && !containsAny(w.block.Bytes(), w.match) {
		return
	}
	// boundedWriter never fails; it reports full writes so errpt keeps draining.
	_, _ = w.kept.Write(w.block.Bytes())
}

// containsAny reports whether b contains one of the strings.
func containsAny(b []byte, subs []string) bool {
	for _, s := range subs {
		if bytes.Contains(b, []byte(s)) {
			return true
		}
	}
	return false
}

// result flushes any pending partial line and block and returns the retained
// output, with a truncation marker if the cap was hit, and whether the raw
// output contained any record at all.
func (w *errptWriter) result() ([]byte, bool) {
	if w.line.Len() > 0 {
		w.block.Write(w.line.Bytes())
		w.block.WriteByte('\n')
	}
	w.flushBlock()
	return w.kept.Bytes(), w.hadRecords
}

// buildErrptLog assembles errpt.log; query errors are reported inline, and the
// root hint is written only when all queries succeeded but saw no record at
// all (errpt is unreadable for non-root, e.g. the agent process).
func buildErrptLog(deaths, paging, src errptResult) []byte {
	content := bytes.NewBuffer(nil)

	writeSection := func(header string, r errptResult) {
		content.WriteString(header)
		content.Write(r.out)
		if r.err != nil {
			content.WriteString(fmt.Sprintf("\n(errpt failed: %v)\n", r.err))
		} else if len(r.out) == 0 && r.hadRecords {
			content.WriteString("(no matching records)\n")
		}
	}

	writeSection("=== Agent process deaths by signal (errpt -a -j A924A5FC) ===\n", deaths)
	content.WriteString("\n\n")
	writeSection("=== Kernel paging space events (errpt -a -j 620096CD,8527F6F4) ===\n", paging)
	content.WriteString("\n\n")
	writeSection("=== Agent SRC subsystem lifecycle (errpt -a -N SRC) ===\n", src)

	if deaths.err == nil && paging.err == nil && src.err == nil &&
		!deaths.hadRecords && !paging.hadRecords && !src.hadRecords {
		content.WriteString("\nerrpt returned no entries. The AIX error log is only readable by root; " +
			"to include it, run `sudo datadog-agent flare --local` so the CLI builds the flare as root " +
			"instead of the agent process.\n")
	}

	return content.Bytes()
}

// boundedWriter keeps the first max bytes written to it and discards the
// rest, while reporting full writes so the command output keeps draining.
type boundedWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	n := min(len(p), max(0, w.max-w.buf.Len()))
	w.buf.Write(p[:n])
	if len(p) > n {
		w.truncated = true
	}
	return len(p), nil
}

// appendTruncationMarker copies out with the truncation marker appended.
func appendTruncationMarker(out []byte) []byte {
	marked := make([]byte, 0, len(out)+len(truncatedMarker))
	marked = append(marked, out...)
	return append(marked, truncatedMarker...)
}

// Bytes returns the retained output, with a truncation marker if any bytes
// were discarded.
func (w *boundedWriter) Bytes() []byte {
	if !w.truncated {
		return w.buf.Bytes()
	}
	return appendTruncationMarker(w.buf.Bytes())
}
