// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python
// +build python

package integrations

import (
	"io"
	"os/exec"
)

// Interface to run external commands.
// This lets us inject a custom runner (e.g. for tests)
type commandRunner interface {
	Output() ([]byte, error)
	Start() error
	StderrPipe() (io.ReadCloser, error)
	StdoutPipe() (io.ReadCloser, error)
	Wait() error
	// We need this additional method to wrap access to the `.Env` field
	SetEnv([]string)
}

type commandConstructor func(name string, arg ...string) commandRunner

type defaultRunner struct {
	*exec.Cmd
}

func (c *defaultRunner) SetEnv(newEnv []string) {
	c.Cmd.Env = newEnv
}

func execCommand(name string, arg ...string) commandRunner {
	cmd := &defaultRunner{}
	cmd.Cmd = exec.Command(name, arg...)
	return commandRunner(cmd)
}
