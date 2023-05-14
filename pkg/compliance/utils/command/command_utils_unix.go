// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func parseID(s string) (uint32, error) {
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}

	if id < 0 || id > math.MaxInt32 {
		return 0, fmt.Errorf("ID for '%s' must be positive and less that %d", s, math.MaxInt32)
	}

	return uint32(id), nil
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

	// Fallback to these values when the nobody user can't be found
	uid := uint32(65534)
	gid := uint32(65534)

	runAs := config.Datadog.GetString("compliance_config.run_commands_as")
	if runAs == "" {
		runAs = "nobody"
	}

	user, err := user.Lookup(runAs)
	if err == nil {
		uid, err = parseID(user.Uid)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get uid for user %s", runAs)
		}

		gid, err = parseID(user.Gid)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to get gid for user %s", runAs)
		}
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}

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
