// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/selinux"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentPackage = "datadog-agent"

	agentSymlink       = "/usr/bin/datadog-agent"
	installerSymlink   = "/usr/bin/datadog-installer"
	legacyAgentSymlink = "/opt/datadog-agent"

	stablePath     = "/opt/datadog-packages/datadog-agent/stable"
	experimentPath = "/opt/datadog-packages/datadog-agent/experiment"

	agentUnit          = "datadog-agent.service"
	installerAgentUnit = "datadog-agent-installer.service"
	traceAgentUnit     = "datadog-agent-trace.service"
	processAgentUnit   = "datadog-agent-process.service"
	systemProbeUnit    = "datadog-agent-sysprobe.service"
	securityAgentUnit  = "datadog-agent-security.service"
	agentExp           = "datadog-agent-exp.service"
	installerAgentExp  = "datadog-agent-installer-exp.service"
	traceAgentExp      = "datadog-agent-trace-exp.service"
	processAgentExp    = "datadog-agent-process-exp.service"
	systemProbeExp     = "datadog-agent-sysprobe-exp.service"
	securityAgentExp   = "datadog-agent-security-exp.service"
)

var (
	stableUnits = []string{
		agentUnit,
		installerAgentUnit,
		traceAgentUnit,
		processAgentUnit,
		systemProbeUnit,
		securityAgentUnit,
	}
	experimentalUnits = []string{
		agentExp,
		installerAgentExp,
		traceAgentExp,
		processAgentExp,
		systemProbeExp,
		securityAgentExp,
	}
)

var (
	// agentDirectories are the directories that the agent needs to function
	agentDirectories = file.Directories{
		{Path: "/etc/datadog-agent", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
		{Path: "/var/log/datadog", Mode: 0755, Owner: "dd-agent", Group: "dd-agent"},
	}

	// agentConfigPermissions are the ownerships and modes that are enforced on the agent configuration files
	agentConfigPermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "managed", Owner: "root", Group: "root", Recursive: true},
		{Path: "inject", Owner: "root", Group: "root", Recursive: true},
		{Path: "compliance.d", Owner: "root", Group: "root", Recursive: true},
		{Path: "runtime-security.d", Owner: "root", Group: "root", Recursive: true},
		{Path: "system-probe.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "system-probe.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "security-agent.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
		{Path: "security-agent.yaml.example", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
	}

	// agentPackagePermissions are the ownerships and modes that are enforced on the agent package files
	agentPackagePermissions = file.Permissions{
		{Path: ".", Owner: "dd-agent", Group: "dd-agent", Recursive: true},
		{Path: "embedded/bin/system-probe", Owner: "root", Group: "root"},
		{Path: "embedded/bin/security-agent", Owner: "root", Group: "root"},
		{Path: "embedded/share/system-probe/ebpf", Owner: "root", Group: "root", Recursive: true},
		{Path: "embedded/share/system-probe/java", Owner: "root", Group: "root", Recursive: true},
	}
)

// PrepareAgent prepares the machine to install the agent
func PrepareAgent(ctx context.Context) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "prepare_agent")
	defer func() { span.Finish(err) }()

	for _, unit := range stableUnits {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
		if err := systemd.DisableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
	}
	return packagemanager.RemovePackage(ctx, agentPackage)
}

// SetupAgent installs and starts the agent
func SetupAgent(ctx context.Context, _ []string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent, reverting: %s", err)
			err = errors.Join(err, RemoveAgent(ctx))
		}
		span.Finish(err)
	}()

	err = PostInstallAgent(ctx, stablePath, "installer")
	if err != nil {
		return err
	}

	err = setupStableUnits(ctx)
	return err
}

// PostInstallAgent performs post-installation steps for the agent
func PostInstallAgent(ctx context.Context, installPath string, caller string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "post_install_agent")
	defer func() {
		span.Finish(err)
	}()

	// 1. Ensure the dd-agent user and group exist
	userHomePath := installPath
	if installPath == stablePath || installPath == experimentPath {
		userHomePath = "/opt/datadog-packages"
	}
	if err = user.EnsureAgentUserAndGroup(ctx, userHomePath); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}

	// 2. Ensures the installer is present in the agent package
	installerPath := filepath.Join(installPath, "embedded", "bin", "installer")
	if _, err := os.Stat(installerPath); os.IsNotExist(err) {
		err = installerCopy(installerPath)
		if err != nil {
			return fmt.Errorf("failed to copy installer: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check installer: %w", err)
	}

	// 3. Ensure config/log/package directories are created and have the correct permissions
	if err = agentDirectories.Ensure(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err = agentPackagePermissions.Ensure(installPath); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	if err = agentConfigPermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set config ownerships: %v", err)
	}

	// 4. Create symlinks
	if err = file.EnsureSymlink(filepath.Join(installPath, "bin/agent/agent"), agentSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}
	if installPath == stablePath {
		if err = file.EnsureSymlink(installPath, legacyAgentSymlink); err != nil {
			return fmt.Errorf("failed to create symlink: %v", err)
		}
	}
	if err = file.EnsureSymlinkIfNotExists(filepath.Join(installPath, "embedded/bin/installer"), installerSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// 5. Set up SELinux permissions
	if err = selinux.SetAgentPermissions("/etc/datadog-agent", installPath); err != nil {
		log.Warnf("failed to set SELinux permissions: %v", err)
	}

	// 6. Handle install info
	if err = installinfo.WriteInstallInfo(caller); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
	}

	// 7. Call post.py for integration persistence. Allowed to fail.
	// XXX: We should port this to Go
	if _, err := os.Stat(filepath.Join(installPath, "embedded/bin/python")); err == nil {
		cmd := exec.Command(filepath.Join(installPath, "embedded/bin/python"), filepath.Join(installPath, "python-scripts/post.py"), installPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed to run integration persistence in post.py: %s\n", err.Error())
		}
	}

	return nil
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) error {
	span, ctx := telemetry.StartSpanFromContext(ctx, "remove_agent_units")
	var spanErr error
	defer func() { span.Finish(spanErr) }()
	// stop, disable, & delete units from disk
	spanErr = removeAgentUnits(ctx, agentExp, true)
	if spanErr != nil {
		log.Warnf("Failed to remove experimental units: %s", spanErr)
	}
	spanErr = removeAgentUnits(ctx, agentUnit, false)
	if spanErr != nil {
		log.Warnf("Failed to remove stable units: %s", spanErr)
	}
	if err := os.Remove(agentSymlink); err != nil && !os.IsNotExist(err) {
		log.Warnf("Failed to remove agent symlink: %s", err)
		spanErr = err
	}
	if err := os.Remove(legacyAgentSymlink); err != nil && !os.IsNotExist(err) {
		log.Warnf("Failed to remove legacy agent symlink: %s", err)
		spanErr = err
	}
	if err := os.Remove(installerSymlink); err != nil && !os.IsNotExist(err) {
		log.Warnf("Failed to remove agent symlink: %s", err)
		spanErr = err
	}
	installinfo.RemoveInstallInfo()
	return nil
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) error {
	if err := PostInstallAgent(ctx, experimentPath, "installer"); err != nil {
		return err
	}
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	err := setupExperimentUnits(ctx)
	return err
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment(ctx context.Context) error {
	if err := PostInstallAgent(ctx, stablePath, "installer"); err != nil {
		return err
	}
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	if err := setupStableUnits(ctx); err != nil {
		return err
	}
	return removeAgentUnits(ctx, agentExp, true)
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(ctx context.Context) error {
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	if err := setupStableUnits(ctx); err != nil {
		return err
	}
	return removeAgentUnits(ctx, agentExp, true)
}

func setupStableUnits(ctx context.Context) error {
	return setupAgentUnits(ctx, agentUnit, stableUnits)
}

func setupExperimentUnits(ctx context.Context) error {
	return setupAgentUnits(ctx, agentExp, experimentalUnits)
}

func removeAgentUnits(ctx context.Context, coreAgentUnit string, experiment bool) error {
	units, err := systemd.ListOnDiskAgentUnits(experiment)
	if err != nil {
		return fmt.Errorf("failed to list agent units: %v", err)
	}

	for _, unit := range units {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			return err
		}
	}

	if err := systemd.DisableUnit(ctx, coreAgentUnit); err != nil {
		return err
	}

	for _, unit := range units {
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			return err
		}
	}
	return nil
}

func setupAgentUnits(ctx context.Context, coreAgentUnit string, units []string) error {
	for _, unit := range units {
		if err := systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}

	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// enabling the core agent unit only is enough as others are triggered by it
	if err := systemd.EnableUnit(ctx, coreAgentUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", coreAgentUnit, err)
	}

	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	} else if os.IsNotExist(err) {
		// this is expected during a fresh install with the install script / ansible / chef / etc...
		// the config is populated afterwards by the install method and the agent is restarted
		return nil
	}
	if err = systemd.StartUnit(ctx, coreAgentUnit); err != nil {
		return err
	}
	return nil
}

func installerCopy(path string) error {
	// Copy the current executable to the installer path
	// This is temporary and will be removed after next release
	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}

	sourceFile, err := os.Open(currentExecutable)
	if err != nil {
		return fmt.Errorf("failed to open current executable: %w", err)
	}
	defer sourceFile.Close()

	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create installer directory: %w", err)
	}
	destinationFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy executable: %w", err)
	}

	err = destinationFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("failed to set permissions on destination file: %w", err)
	}
	return nil
}
