// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func ValueFromShellCommand(command string, shellAndArgs ...string) (interface{}, error) {
	log.Debugf("Resolving value from shell command: %s, args [%s]", command, strings.Join(shellAndArgs, ","))

	shellCmd := &compliance.ShellCmd{
		Run: command,
	}
	if len(shellAndArgs) > 0 {
		shellCmd.Shell = &compliance.BinaryCmd{
			Name: shellAndArgs[0],
			Args: shellAndArgs[1:],
		}
	}
	execCommand := shellCmdToBinaryCmd(shellCmd)
	exitCode, stdout, err := runBinaryCmd(execCommand, compliance.DefaultTimeout)
	if exitCode != 0 || err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", command, err)
	}
	return stdout, nil
}

func ValueFromBinaryCommand(name string, args ...string) (interface{}, error) {
	log.Debugf("Resolving value from command: %s, args [%s]", name, strings.Join(args, ","))
	execCommand := &compliance.BinaryCmd{
		Name: name,
		Args: args,
	}
	exitCode, stdout, err := runBinaryCmd(execCommand, compliance.DefaultTimeout)
	if exitCode != 0 || err != nil {
		return nil, fmt.Errorf("command '%v' execution failed, error: %v", execCommand, err)
	}
	return stdout, nil
}
