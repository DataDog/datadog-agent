// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package flare

import (
	"context"
	"os"
	"os/exec"
	"strconv"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// getSvmonData captures the AIX virtual memory segment breakdown (heap,
// stack, text, shared library segments) for the running Agent process, since
// resource limits are enforced per segment and RSS/VSZ alone don't show which
// segment is close to its ulimit.
func getSvmonData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	pid := strconv.Itoa(os.Getpid())
	cmd := exec.CommandContext(ctx, "svmon", "-P", pid, "-O", "unit=KB,segment=on")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = fb.Logf("error running svmon -P %s: %s", pid, err)
	}

	return fb.AddFile("svmon.log", out)
}
