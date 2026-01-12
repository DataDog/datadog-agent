// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package selinux offers an interface to set agent's SELinux permissions.
package selinux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	gopsutilhost "github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

const manualInstallTemplate = `To be able to run system-probe on your host, please install or update the selinux-policy-targeted and
policycoreutils-python (or policycoreutils-python-utils depending on your distribution) packages.
Then run the following commands, or reinstall datadog-agent:
	semodule -i %[1]s/selinux/system_probe_policy.pp
	semanage fcontext -a -t system_probe_t %[2]s/embedded/bin/system-probe
	semanage fcontext -a -t system_probe_t %[2]s/bin/agent/agent
	restorecon -v %[2]s/embedded/bin/system-probe %[2]s/bin/agent/agent
`

// SetAgentPermissions sets the SELinux permissions for the agent if the OS requires it.
func SetAgentPermissions(ctx context.Context, configPath, installPath string) (err error) {
	span, _ := telemetry.StartSpanFromContext(ctx, "selinux_set_agent_permissions")
	defer func() {
		span.Finish(err)
	}()

	shouldSet, err := isSELinuxSupported()
	if err != nil {
		return fmt.Errorf("error checking if SELinux is supported: %w", err)
	}
	if !shouldSet {
		return nil
	}

	// Load the SELinux policy module for the agent
	fmt.Println("Loading SELinux policy module for datadog-agent.")
	cmd := telemetry.CommandContext(ctx, "semodule", "-v", "-i", filepath.Join(configPath, "selinux/system_probe_policy.pp"))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Couldn't load system-probe policy (%v).\n", err)
		printManualInstructions(configPath, installPath)
		return fmt.Errorf("couldn't load system-probe policy: %v", err)
	}

	// Check if semanage / restorecon are available
	if !isInstalled("semanage") || !isInstalled("restorecon") {
		fmt.Println("Couldn't load system-probe policy (missing selinux utilities).")
		printManualInstructions(configPath, installPath)
		return errors.New("missing selinux utilities")
	}

	// Label the system-probe binary
	fmt.Println("Labeling SELinux type for the system-probe binary.")
	cmd = telemetry.CommandContext(ctx, "semanage", "fcontext", "-a", "-t", "system_probe_t", filepath.Join(installPath, "embedded/bin/system-probe"))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Couldn't install system-probe policy (%v).\n", err)
		printManualInstructions(configPath, installPath)
		return fmt.Errorf("couldn't install system-probe policy: %v", err)
	}
	cmd = telemetry.CommandContext(ctx, "semanage", "fcontext", "-a", "-t", "system_probe_t", filepath.Join(installPath, "bin/agent/agent"))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Couldn't install system-probe policy (%v).\n", err)
		printManualInstructions(configPath, installPath)
		return fmt.Errorf("couldn't install system-probe policy: %v", err)
	}
	cmd = telemetry.CommandContext(ctx, "restorecon", "-v", filepath.Join(installPath, "embedded/bin/system-probe"))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Couldn't install system-probe policy (%v).\n", err)
		printManualInstructions(configPath, installPath)
		return fmt.Errorf("couldn't install system-probe policy: %v", err)
	}
	cmd = telemetry.CommandContext(ctx, "restorecon", "-v", filepath.Join(installPath, "bin/agent/agent"))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Couldn't install system-probe policy (%v).\n", err)
		printManualInstructions(configPath, installPath)
		return fmt.Errorf("couldn't install system-probe policy: %v", err)
	}

	return nil
}

func printManualInstructions(configPath, installPath string) {
	fmt.Printf(manualInstallTemplate, configPath, installPath)
}

// isSelinuxSupported checks if SELinux is supported on the host,
// ie if the host is on RHEL 7
func isSELinuxSupported() (bool, error) {
	_, family, version, err := gopsutilhost.PlatformInformation()
	if err != nil {
		return false, fmt.Errorf("error getting platform information: %w", err)
	}
	return (family == "rhel" && strings.HasPrefix(version, "7") && isInstalled("semodule")), nil
}

func isInstalled(program string) bool {
	_, err := exec.LookPath(program)
	return err == nil
}
