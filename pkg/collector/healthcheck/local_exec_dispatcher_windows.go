// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package healthcheck

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
)

func configureLocalCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), localWaitDelay)
		defer cancel()
		kill := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.WaitDelay = localWaitDelay
		if err := kill.Run(); err != nil {
			return errors.Join(err, cmd.Process.Kill())
		}
		return nil
	}
}
