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
	// errptMaxOutput caps each errpt section, both in the flare and in memory.
	errptMaxOutput = 256 * 1024
	// errptMaxErrOutput caps the stderr kept in error messages.
	errptMaxErrOutput = 4 * 1024
)

const truncatedMarker = "\n[output truncated]\n"

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

// getErrptData collects the AIX error log for the last 7 days: process deaths
// by signal, kernel paging-space kills, and SRC subsystem restarts — often the
// only in-host evidence when the agent dies without leaving anything in its
// own logs. It also runs in local mode (the data is host-wide), because errpt
// is only readable by root: the agent process (dd-agent) cannot include it,
// and errpt.log then carries a hint to rerun with
// `sudo datadog-agent flare --local`, which builds the flare as root.
func getErrptData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	since := errptSince(time.Now().Add(-errptWindow))

	deaths, errDeaths := runErrpt(ctx, "-a", "-j", "A924A5FC,620096CD,8527F6F4", "-s", since)
	if errDeaths != nil {
		_ = fb.Logf("error running errpt for process deaths: %s", errDeaths)
	}
	src, errSrc := runErrpt(ctx, "-a", "-N", "SRC", "-s", since)
	if errSrc != nil {
		_ = fb.Logf("error running errpt for SRC entries: %s", errSrc)
	}

	return fb.AddFile("errpt.log", buildErrptLog(deaths, errDeaths, src, errSrc))
}

// errptSince formats t in the MMDDhhmmYY form expected by errpt's -s flag.
func errptSince(t time.Time) string {
	return t.Format("0102150406")
}

// runErrpt returns errpt's stdout only, so query errors are never mistaken for
// error-log content; stderr is folded into the returned error.
func runErrpt(ctx context.Context, args ...string) ([]byte, error) {
	stdout := &boundedWriter{max: errptMaxOutput}
	stderr := &boundedWriter{max: errptMaxErrOutput}
	cmd := exec.CommandContext(ctx, "errpt", args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		if stderr.buf.Len() > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(stderr.Bytes())))
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

// buildErrptLog assembles errpt.log; query errors are reported inline, and the
// root hint is written only when both queries succeeded and returned nothing.
func buildErrptLog(deaths []byte, errDeaths error, src []byte, errSrc error) []byte {
	content := bytes.NewBuffer(nil)

	content.WriteString("=== Process deaths by signal, core dumps and paging space events (errpt -a -j A924A5FC,620096CD,8527F6F4) ===\n")
	content.Write(deaths)
	if errDeaths != nil {
		content.WriteString(fmt.Sprintf("\n(errpt failed: %v)\n", errDeaths))
	}

	content.WriteString("\n\n=== SRC subsystem lifecycle (errpt -a -N SRC) ===\n")
	content.Write(src)
	if errSrc != nil {
		content.WriteString(fmt.Sprintf("\n(errpt failed: %v)\n", errSrc))
	}

	if errDeaths == nil && errSrc == nil && len(deaths) == 0 && len(src) == 0 {
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
