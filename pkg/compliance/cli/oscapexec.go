// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/syndtr/gocapability/capability"
	"golang.org/x/sys/unix"
)

// RunOscapExec executes oscap-io with dropped capabilities
func RunOscapExec(args []string) error {
	binaryPath, err := getOSCAPIODefaultBinPath()
	if err != nil {
		return err
	}

	// Verify the binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("oscap-io binary not found at %s: %w", binaryPath, err)
	}

	// Drop capabilities
	if err := dropCapabilities(); err != nil {
		return fmt.Errorf("failed to drop capabilities: %w", err)
	}

	// Set PR_SET_NO_NEW_PRIVS to prevent privilege escalation
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("failed to set PR_SET_NO_NEW_PRIVS: %w", err)
	}

	// Execute oscap-io (replaces current process)
	// Note: syscall.Exec never returns on success
	execArgs := append([]string{binaryPath}, args...)
	if err := syscall.Exec(binaryPath, execArgs, os.Environ()); err != nil {
		return fmt.Errorf("failed to exec oscap-io at %s: %w", binaryPath, err)
	}

	// This line should never be reached
	return nil
}

func getOSCAPIODefaultBinPath() (string, error) {
	here, err := executable.Folder()
	if err != nil {
		return "", err
	}

	binPath := filepath.Join(here, "..", "..", "embedded", "bin", "oscap-io")
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}
	return binPath, fmt.Errorf("can't access the default oscap-io binary at %s", binPath)
}

// dropCapabilities drops all capabilities except CAP_SYS_CHROOT and CAP_DAC_OVERRIDE
func dropCapabilities() error {
	// Create a new capability set for the current process
	caps, err := capability.NewPid2(0)
	if err != nil {
		return fmt.Errorf("failed to create capability set: %w", err)
	}

	// Load current capabilities
	if err := caps.Load(); err != nil {
		return fmt.Errorf("failed to load capabilities: %w", err)
	}

	// Clear all capabilities in all sets
	caps.Clear(capability.CAPS)

	// Set CAP_SYS_CHROOT and CAP_DAC_OVERRIDE in all capability sets
	caps.Set(capability.CAPS, capability.CAP_SYS_CHROOT, capability.CAP_DAC_OVERRIDE)

	// Apply the capability changes
	if err := caps.Apply(capability.CAPS); err != nil {
		return fmt.Errorf("failed to apply capability changes: %w", err)
	}

	return nil
}
