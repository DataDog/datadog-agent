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
}

type commandConstructor func(name string, arg ...string) commandRunner

func execCommand(name string, arg ...string) commandRunner {
	return commandRunner(exec.Command(name, arg...))
}
