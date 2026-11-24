// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package exec provides an implementation of the Installer interface that uses the installer binary.
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

const (
	defaultGetStatesTimeout   = 30 * time.Second
	defaultGetStatesWaitDelay = 15 * time.Second
)

func (i *InstallerExec) newInstallerCmdPlatform(cmd *exec.Cmd) *exec.Cmd {
	// os.Interrupt is not support on Windows
	// It gives " run failed: exec: canceling Cmd: not supported by windows"
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}

	return cmd
}

// getStates retrieves the state of all packages & their configuration from disk.
// On Linux/macOS this spawns a subprocess for privilege escalation.
func (i *InstallerExec) getStates(ctx context.Context) (repo *repository.PackageStates, err error) {
	return i.getStatesWithTimeout(ctx, defaultGetStatesTimeout, defaultGetStatesWaitDelay)
}

func (i *InstallerExec) getStatesWithTimeout(ctx context.Context, timeout, waitDelay time.Duration) (repo *repository.PackageStates, err error) {
	stateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := i.newInstallerCmd(stateCtx, "get-states")
	// Bound how long graceful cancellation may take. The installer waits 10 seconds
	// after SIGINT before cancelling its work, so allow that handler to complete
	// before os/exec forcibly kills the child and closes its pipes.
	cmd.WaitDelay = waitDelay
	defer func() { cmd.span.Finish(err) }()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		if ctxErr := stateCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("error getting state from disk: %w: %v\n%s", ctxErr, err, stderr.String())
		}
		return nil, fmt.Errorf("error getting state from disk: %w\n%s", err, stderr.String())
	}
	var pkgStates *repository.PackageStates
	err = json.Unmarshal(stdout.Bytes(), &pkgStates)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling state from disk: %w\n`%s`", err, stdout.String())
	}
	return pkgStates, nil
}
