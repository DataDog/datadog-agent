// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package exec

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func (i *InstallerExec) newCmd(_ context.Context, cmd *exec.Cmd, command string, args []string) *exec.Cmd {
	// os.Interrupt is not supported on Windows
	// It gives " run failed: exec: canceling Cmd: not supported by windows"
	escapedBinPath := fmt.Sprintf(`"%s"`, strings.ReplaceAll(i.installerBinPath, "/", `\`))
	cmd = exec.Command(i.installerBinPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: strings.Join(append([]string{escapedBinPath, command}, args...), " ")}
	return cmd
}
