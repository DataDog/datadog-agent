// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package flare

import (
	"context"
	"os/exec"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

func getWindowsData(_ context.Context, _ flaretypes.FlareBuilder) error {
	return nil
}

// getUlimitData captures the resource limits in effect for the running Agent
// process. `ulimit` is a shell builtin, so a subshell is spawned to run it;
// that subshell inherits the Agent process' own limits via fork(), so the
// output reflects the Agent's actual limits, not an unrelated login shell's.
// Both the soft (currently enforced) and hard (ceiling) limits are captured.
func getUlimitData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	cmd := exec.CommandContext(ctx, "sh", "-c",
		`echo "=== Soft limits (ulimit -aS) ==="; ulimit -aS; echo; echo "=== Hard limits (ulimit -aH) ==="; ulimit -aH`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = fb.Logf("error running ulimit: %s", err)
	}

	return fb.AddFile("ulimit.log", out)
}
