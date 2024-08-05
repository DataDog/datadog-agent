// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type commandRunner interface {
	run() error
	setStderr(stderr *bytes.Buffer)
}

type realCmd struct {
	*exec.Cmd
}

type mockCmd struct {
	runFunc    func() error
	stderrData string
}

func (r *realCmd) run() error {
	return r.Cmd.Run()
}

func (r *realCmd) setStderr(stderr *bytes.Buffer) {
	r.Cmd.Stderr = stderr
}

func (m *mockCmd) run() error {
	return m.runFunc()
}

func (m *mockCmd) setStderr(stderr *bytes.Buffer) {
	stderr.WriteString(m.stderrData)
}

func newCommandRunner(ctx context.Context, name string, arg ...string) commandRunner {
	cmd := exec.CommandContext(ctx, name, arg...)
	return &realCmd{Cmd: cmd}
}

func runCommand(cmdR commandRunner) error {
	var stderr bytes.Buffer
	cmdR.setStderr(&stderr)

	err := cmdR.run()
	if err == nil {
		return nil
	}

	if _, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("command failed: %s", stderr.String())
	}
	if baseError, ok := err.(*exec.Error); ok {
		return fmt.Errorf("command failed: %s", baseError.Error())
	}
	return fmt.Errorf("command failed: %s", err.Error())
}
