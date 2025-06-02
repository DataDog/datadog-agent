// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"os"
	"os/exec"
)

func (i *InstallerExec) newInstallerCmdPlatform(cmd *exec.Cmd) *exec.Cmd {
	// os.Interrupt is not support on Windows
	// It gives " run failed: exec: canceling Cmd: not supported by windows"
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}

	return cmd
}
