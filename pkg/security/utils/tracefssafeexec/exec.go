// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tracefssafeexec provides utilities to safely execute commands, without racing with tracefs operations
package tracefssafeexec

import (
	"context"
	"os/exec"

	manager "github.com/DataDog/ebpf-manager"
)

// Cmd wraps exec.Cmd to provide tracefs-safe command execution. This should really only be used for very fast commands,
// as it holds a global lock during the command execution.
type Cmd struct {
	*exec.Cmd
}

// Command returns a new Cmd to execute the named program with the given arguments
func Command(name string, arg ...string) *Cmd {
	return &Cmd{exec.Command(name, arg...)}
}

// CommandContext returns a new Cmd to execute the named program with the given arguments and context
func CommandContext(ctx context.Context, name string, arg ...string) *Cmd {
	return &Cmd{exec.CommandContext(ctx, name, arg...)}
}

// Start starts the specified command but does not wait for it to complete while holding the TraceFSLock
func (c *Cmd) Start() error {
	manager.TraceFSLock.Lock()
	defer manager.TraceFSLock.Unlock()
	return c.Cmd.Start()
}

// Run starts the specified command and waits for it to complete while holding the TraceFSLock
func (c *Cmd) Run() error {
	manager.TraceFSLock.Lock()
	defer manager.TraceFSLock.Unlock()
	return c.Cmd.Run()
}

// CombinedOutput runs the command and returns its combined standard output and standard error while holding the TraceFSLock
func (c *Cmd) CombinedOutput() ([]byte, error) {
	manager.TraceFSLock.Lock()
	defer manager.TraceFSLock.Unlock()
	return c.Cmd.CombinedOutput()
}

// Output runs the command and returns its standard output while holding the TraceFSLock
func (c *Cmd) Output() ([]byte, error) {
	manager.TraceFSLock.Lock()
	defer manager.TraceFSLock.Unlock()
	return c.Cmd.Output()
}
