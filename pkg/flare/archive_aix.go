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

const (
	// errptWindow is how far back the collected error log entries go.
	errptWindow = 7 * 24 * time.Hour
)

// getErrptData collects the AIX error log for the last 7 days: process
// deaths by signal (with the core file path), kernel paging-space kills, and
// SRC subsystem restarts. This is often the only in-host evidence when the
// agent dies without leaving anything in its own logs (silent crash or
// SIGKILL).
//
// Unlike the other collectors, this one runs in local mode too: the data is
// host-wide, not agent-process state. errpt only shows entries to root, so a
// flare built by the agent process (running as dd-agent) cannot include
// them; errpt.log then carries a hint to rerun with
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

// runErrpt runs errpt and returns its stdout only, so query errors are
// never mistaken for error-log content; stderr is folded into the returned
// error instead.
func runErrpt(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "errpt", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil && stderr.Len() > 0 {
		return stdout.Bytes(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	} else if err != nil {
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}
