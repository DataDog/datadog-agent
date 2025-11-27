// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"os/exec"
	"syscall"
)

// cmdRunner interface wraps the functionality we need from exec.Cmd for testing
type cmdRunner interface {
	Run(path string, cmdLine string) error
}

// realCmdRunner wraps exec.Cmd for production use
type realCmdRunner struct{}

// Run executes the command, creating a new exec.Cmd each time
func (r *realCmdRunner) Run(path string, cmdLine string) error {
	cmd := &exec.Cmd{
		Path: path,
		SysProcAttr: &syscall.SysProcAttr{
			CmdLine: cmdLine,
		},
	}
	return cmd.Run()
}

// newRealCmdRunner creates a cmdRunner that will execute commands using exec.Cmd
func newRealCmdRunner() cmdRunner {
	return &realCmdRunner{}
}
