// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/compliance"
)

var (
	commandRunnerFunc func(context.Context, string, []string, bool) (int, []byte, error) = runCommand
)

func getDefaultShell() compliance.BinaryCmd {
	switch runtime.GOOS {
	case "windows":
		return compliance.BinaryCmd{
			Name: "powershell",
			Args: []string{"-Command"},
		}
	default:
		return compliance.BinaryCmd{
			Name: "sh",
			Args: []string{"-c"},
		}
	}
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
