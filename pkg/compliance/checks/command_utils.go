// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
)

type commandRunnerFunc func(context.Context, string, []string, bool) (int, []byte, error)

var (
	commandRunner commandRunnerFunc = runCommand
)

func getDefaultShell() *compliance.BinaryCmd {
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

func shellCmdToBinaryCmd(shellCmd *compliance.ShellCmd) *compliance.BinaryCmd {
	var execCmd *compliance.BinaryCmd
	if shellCmd.Shell != nil {
		execCmd = shellCmd.Shell
	} else {
		execCmd = getDefaultShell()
	}

	execCmd.Args = append(execCmd.Args, shellCmd.Run)
	return execCmd
}

func runBinaryCmd(execCommand *compliance.BinaryCmd, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	exitCode, stdout, err := commandRunner(ctx, execCommand.Name, execCommand.Args, true)
	return exitCode, string(stdout), err
}
