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
	// errptMaxCollect bounds the raw output collected from one errpt call.
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
	// hadRecords reports whether the unfiltered output had any record.
	hadRecords bool
	err        error
}

// runErrpt runs errpt; when match is not nil, only records containing one of
// the strings are kept. stderr is folded into err so query errors are never
// mistaken for error-log content.
func runErrpt(ctx context.Context, match []string, args ...string) errptResult {
	stdout := &boundedWriter{max: errptMaxCollect}
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
	res.out = stdout.Bytes()

	if match == nil {
		res.hadRecords = len(bytes.TrimSpace(res.out)) > 0
		return res
	}
	res.out, res.hadRecords = filterErrptRecords(res.out, match)
	return res
}

// filterErrptRecords keeps the records that contain one of the match strings, e.g. the
// Agent's binary or subsystem names. Returns the kept records and whether the input
// had any record at all.
func filterErrptRecords(out []byte, match []string) ([]byte, bool) {
	var kept, block bytes.Buffer
	hadRecords := false

	flush := func() {
		if len(bytes.Trim(block.Bytes(), "-\n")) == 0 {
			block.Reset()
			return
		}
		hadRecords = true
		for _, m := range match {
			if bytes.Contains(block.Bytes(), []byte(m)) {
				kept.Write(block.Bytes())
				break
			}
		}
		block.Reset()
	}

	for _, line := range bytes.Split(out, []byte("\n")) {
		block.Write(line)
		block.WriteByte('\n')
		if len(line) > 0 && len(bytes.Trim(line, "-")) == 0 {
			flush()
		}
	}
	flush()

	return kept.Bytes(), hadRecords
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

// Bytes returns the retained output, with a truncation marker if any bytes
// were discarded.
func (w *boundedWriter) Bytes() []byte {
	out := w.buf.Bytes()
	if !w.truncated {
		return out
	}
	capped := make([]byte, 0, len(out)+len(truncatedMarker))
	capped = append(capped, out...)
	return append(capped, truncatedMarker...)
}
