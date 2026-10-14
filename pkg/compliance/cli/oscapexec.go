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

// oscapIOCapabilities are the capabilities oscap-io keeps when the process holds
// them: CAP_DAC_OVERRIDE to read files whatever their mode, CAP_SYS_CHROOT for the
// probes that enter OSCAP_PROBE_ROOT, and CAP_NET_ADMIN and CAP_NET_RAW for the
// SCE checks that read the firewall with ufw, nft and iptables.
var oscapIOCapabilities = []capability.Cap{
	capability.CAP_DAC_OVERRIDE,
	capability.CAP_NET_ADMIN,
	capability.CAP_NET_RAW,
	capability.CAP_SYS_CHROOT,
}

// dropCapabilities reduces the effective, permitted and inheritable sets to the
// oscapIOCapabilities the process holds, since capset can only shrink the
// permitted set.
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

	// Keep the oscapIOCapabilities the process holds
	var keep []capability.Cap
	for _, c := range oscapIOCapabilities {
		if caps.Get(capability.PERMITTED, c) {
			keep = append(keep, c)
		}
	}

	// Clear all capabilities in all sets
	caps.Clear(capability.CAPS)

	// Set the kept capabilities in all capability sets
	caps.Set(capability.CAPS, keep...)

	// Apply the capability changes
	if err := caps.Apply(capability.CAPS); err != nil {
		return fmt.Errorf("failed to apply capability changes: %w", err)
	}

	return nil
}
