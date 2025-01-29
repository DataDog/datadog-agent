// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package exec

import (
	"context"
	"os"
	"os/exec"
)

func (i *InstallerExec) newCmd(ctx context.Context, command string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, i.installerBinPath, append([]string{command}, args...)...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	return cmd
}
