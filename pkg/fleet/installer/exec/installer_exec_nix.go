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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
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
	cmd := i.newInstallerCmd(ctx, "get-states")
	defer func() { cmd.span.Finish(err) }()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error getting state from disk: %w\n%s", err, stderr.String())
	}
	var pkgStates *repository.PackageStates
	err = json.Unmarshal(stdout.Bytes(), &pkgStates)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling state from disk: %w\n`%s`", err, stdout.String())
	}
	return pkgStates, nil
}
