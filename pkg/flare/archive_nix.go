// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package flare

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getWindowsData(_ context.Context, _ flaretypes.FlareBuilder) error {
	return nil
}

// ulimitTimeout bounds the `ulimit -a` subshell invocation.
const ulimitTimeout = 10 * time.Second

// getUlimitData captures the resource limits in effect for the running Agent
// process. `ulimit` is a shell builtin, so a subshell is spawned to run it;
// that subshell inherits the Agent process' own limits via fork(), so the
// output reflects the Agent's actual limits, not an unrelated login shell's.
func getUlimitData(ctx context.Context, fb flaretypes.FlareBuilder) error {
	cancelctx, cancelfunc := context.WithTimeout(ctx, ulimitTimeout)
	defer cancelfunc()

	cmd := exec.CommandContext(cancelctx, "sh", "-c", "ulimit -a")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		log.Errorf("error running ulimit -a: %s", err)
	}

	return fb.AddFile("ulimit.log", out.Bytes())
}
