// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
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

func runCommand(ctx context.Context, name string, args []string, captureStdout bool) (int, []byte, error) {
	if len(name) == 0 {
		return 0, nil, errors.New("cannot run empty command")
	}

	_, err := exec.LookPath(name)
	if err != nil {
		return 0, nil, fmt.Errorf("command '%s' not found, err: %v", name, err)
	}

	cmd := exec.CommandContext(ctx, name, args...)
	if cmd == nil {
		return 0, nil, errors.New("unable to create command context")
	}

	var stdoutBuffer bytes.Buffer
	if captureStdout {
		cmd.Stdout = &stdoutBuffer
	}

	err = cmd.Run()

	// We expect ExitError as commands may have an exitCode != 0
	// It's not a failure for a compliance command
	var e *exec.ExitError
	if errors.As(err, &e) {
		err = nil
	}

	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), stdoutBuffer.Bytes(), err
	}
	return -1, nil, fmt.Errorf("unable to retrieve exit code, err: %v", err)
}
