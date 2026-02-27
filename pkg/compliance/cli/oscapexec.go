// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cli

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/syndtr/gocapability/capability"
	"golang.org/x/sys/unix"
)

// RunOscapExec executes oscap-io with dropped capabilities
func RunOscapExec(args []string) error {
	if len(args) < 1 {
		return errors.New("oscap-exec requires at least one argument (binary path)")
	}

	binaryPath := args[0]
	execArgs := args // args[0] will be the binary path (used as argv[0])

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
	if err := syscall.Exec(binaryPath, execArgs, os.Environ()); err != nil {
		return fmt.Errorf("failed to exec oscap-io at %s: %w", binaryPath, err)
	}

	// This line should never be reached
	return nil
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
