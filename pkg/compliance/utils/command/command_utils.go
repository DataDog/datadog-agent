// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"context"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
)

// RunnerFunc describes a function in charge of executing commands
type RunnerFunc func(context.Context, string, []string, bool) (int, []byte, error)

var (
	// Runner holds the singleton for command execution
	Runner RunnerFunc = runCommand
)

// GetDefaultShell returns the shell for the current environment
func GetDefaultShell() *compliance.BinaryCmd {
	switch runtime.GOOS {
	case "windows":
		return &compliance.BinaryCmd{
			Name: "powershell",
			Args: []string{"-Command"},
		}
	default:
		return &compliance.BinaryCmd{
			Name: "sh",
			Args: []string{"-c"},
		}
	}
}

// ShellCmdToBinaryCmd returns the passed command to be executed by a shell command
func ShellCmdToBinaryCmd(shellCmd *compliance.ShellCmd) *compliance.BinaryCmd {
	var execCmd *compliance.BinaryCmd
	if shellCmd.Shell != nil {
		execCmd = shellCmd.Shell
	} else {
		execCmd = GetDefaultShell()
	}

	execCmd.Args = append(execCmd.Args, shellCmd.Run)
	return execCmd
}

// RunBinaryCmd executes the specific command and timeout, returing its return code and output
func RunBinaryCmd(execCommand *compliance.BinaryCmd, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	exitCode, stdout, err := Runner(ctx, execCommand.Name, execCommand.Args, true)
	return exitCode, string(stdout), err
}
