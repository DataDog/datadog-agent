// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package flare

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
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

// getErrptData collects the AIX error log for the last 7 days: process
// deaths by signal (with the core file path), kernel paging-space kills, and
// SRC subsystem restarts. This is often the only in-host evidence when the
// agent dies without leaving anything in its own logs (silent crash or
// SIGKILL).
//
// Unlike the other collectors, this one runs in local mode too: the data is
// host-wide, not agent-process state. errpt only shows entries to root
// (non-root users get an empty log), so a flare built by the agent process
// (running as dd-agent) cannot include it; the file then carries a hint to
// rerun with `sudo datadog-agent flare --local`, which builds the flare from
// the CLI as root.
func getErrptData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	since := time.Now().AddDate(0, 0, -7).Format("0102150406") // errpt -s takes MMDDhhmmYY

	deaths, err := runErrpt(ctx, "-a", "-j", "A924A5FC,620096CD,8527F6F4", "-s", since)
	if err != nil {
		_ = fb.Logf("error running errpt for process deaths: %s", err)
	}
	src, err := runErrpt(ctx, "-a", "-N", "SRC", "-s", since)
	if err != nil {
		_ = fb.Logf("error running errpt for SRC entries: %s", err)
	}

	content := bytes.NewBuffer(nil)
	content.WriteString("=== Process deaths by signal, core dumps and paging space events (errpt -a -j A924A5FC,620096CD,8527F6F4) ===\n")
	content.Write(deaths)
	content.WriteString("\n\n=== SRC subsystem lifecycle (errpt -a -N SRC) ===\n")
	content.Write(src)
	if len(deaths) == 0 && len(src) == 0 {
		content.WriteString("\nerrpt returned no entries. The AIX error log is only readable by root; " +
			"to include it, run `sudo datadog-agent flare --local` so the CLI builds the flare as root " +
			"instead of the agent process.\n")
	}

	return fb.AddFile("errpt.log", content.Bytes())
}

func runErrpt(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "errpt", args...).CombinedOutput()
}
