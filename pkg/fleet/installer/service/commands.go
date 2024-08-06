// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type commandRunner interface {
	run() ([]byte, error)
}

type realCmd struct {
	*exec.Cmd
}

func (r *realCmd) run() ([]byte, error) {
	return r.Cmd.CombinedOutput()
}

func newCommandRunner(ctx context.Context, name string, args ...string) commandRunner {
	cmd := exec.CommandContext(ctx, name, args...)
	return &realCmd{Cmd: cmd}
}

func runCommand(cmdR commandRunner) error {
	output, err := cmdR.run()
	if err == nil {
		return nil
	}

	if len(output) == 0 {
		return fmt.Errorf("command failed: %s", err.Error())
	}

	return fmt.Errorf("command failed: %s \n%s", strings.TrimSpace(string(output)), err.Error())
}
