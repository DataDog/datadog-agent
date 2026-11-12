// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package selinux offers an interface to set agent's SELinux permissions.
package selinux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// SetRuntimePermissions labels runtime directories independently of the legacy binary policy.
func SetRuntimePermissions(ctx context.Context, paths ...string) error {
	enabled, err := runtimeSELinuxEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	persistent := isInstalled("semanage") && isInstalled("restorecon")
	if !persistent && !isInstalled("chcon") {
		return errors.New("SELinux runtime labeling skipped: install semanage and restorecon, or chcon")
	}
	var definitions string
	if persistent {
		output, err := runtimeCommand(ctx, "semanage", "fcontext", "-l", "-C")
		if err != nil {
			return err
		}
		definitions = output
	} else {
		fmt.Println("SELinux runtime labeling uses chcon: install semanage and restorecon to preserve labels across filesystem relabels.")
	}
	var errs []error
	for _, path := range paths {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("resolve runtime directory %s: %w", path, err))
			continue
		}
		if persistent {
			rule := runtimeContextRule(resolved)
			currentType := runtimeContextType(definitions, rule)
			if currentType != "var_run_t" {
				operation := "-a"
				if currentType != "" {
					operation = "-m"
				}
				if _, err := runtimeCommand(ctx, "semanage", "fcontext", operation, "-t", "var_run_t", rule); err != nil {
					errs = append(errs, err)
					continue
				}
				definitions += "\n" + rule + " all files system_u:object_r:var_run_t:s0\n"
			}
			_, err = runtimeCommand(ctx, "restorecon", "-R", resolved)
		} else {
			_, err = runtimeCommand(ctx, "chcon", "-R", "-t", "var_run_t", resolved)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func runtimeSELinuxEnabled(ctx context.Context) (bool, error) {
	if isInstalled("getenforce") {
		output, err := runtimeCommand(ctx, "getenforce")
		if err != nil {
			return false, err
		}
		switch strings.TrimSpace(output) {
		case "Enforcing", "Permissive":
			return true, nil
		case "Disabled":
			return false, nil
		default:
			return false, fmt.Errorf("unknown SELinux state: %q", output)
		}
	}
	if isInstalled("selinuxenabled") {
		err := telemetry.CommandContext(ctx, "selinuxenabled").WithExpectedExitCodes(1).Run()
		if err == nil {
			return true, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("detect SELinux: %w", err)
	}
	return false, errors.New("SELinux runtime labeling skipped: neither getenforce nor selinuxenabled is available")
}

func runtimeCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := telemetry.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var output bytes.Buffer
	cmd.Stdout = &output
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s %v: %w: %s", name, args, err, output.String())
	}
	return output.String(), nil
}

func runtimeContextRule(path string) string {
	const fleetRoot = "/opt/datadog-packages/datadog-agent/"
	if strings.HasPrefix(path, fleetRoot) {
		parts := strings.Split(strings.TrimPrefix(path, fleetRoot), "/")
		if len(parts) == 2 && parts[1] == "run" {
			return fleetRoot + "[^/]+/run(/.*)?"
		}
	}
	return regexp.QuoteMeta(path) + "(/.*)?"
}

func runtimeContextType(definitions, rule string) string {
	for _, line := range strings.Split(definitions, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != rule {
			continue
		}
		context := strings.Split(fields[len(fields)-1], ":")
		if len(context) >= 3 {
			return context[2]
		}
	}
	return ""
}
