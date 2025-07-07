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

// CmdRunner interface wraps the functionality we need from exec.Cmd for testing
type CmdRunner interface {
	Run(path string, cmdLine string) error
}

// RealCmdRunner wraps exec.Cmd for production use
type RealCmdRunner struct{}

// Run executes the command, creating a new exec.Cmd each time
func (r *RealCmdRunner) Run(path string, cmdLine string) error {
	cmd := &exec.Cmd{
		Path: path,
		SysProcAttr: &syscall.SysProcAttr{
			CmdLine: cmdLine,
		},
	}
	return cmd.Run()
}

// NewRealCmdRunner creates a real CmdRunner
func NewRealCmdRunner() CmdRunner {
	return &RealCmdRunner{}
}
