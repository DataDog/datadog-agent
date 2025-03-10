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
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/file"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentPackage       = "datadog-agent"
	agentSymlink       = "/usr/bin/datadog-agent"
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
		{Path: "security-agent.yaml", Owner: "dd-agent", Group: "dd-agent", Mode: 0440},
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

	// 1. Ensure the dd-agent user and group exist
	if err = user.EnsureAgentUserAndGroup(ctx); err != nil {
		return fmt.Errorf("failed to create dd-agent user and group: %v", err)
	}
	// 2. Add install info metadata if it doesn't exist
	if err = installinfo.WriteInstallInfo("manual_update"); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
	}
	// 3. Ensure config/log/package directories are created and have the correct permissions
	if err = agentDirectories.Ensure(); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}
	if err = agentPackagePermissions.Ensure("/opt/datadog-packages/datadog-agent/stable"); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	if err = agentConfigPermissions.Ensure("/etc/datadog-agent"); err != nil {
		return fmt.Errorf("failed to set config ownerships: %v", err)
	}
	if err = file.EnsureSymlink("/opt/datadog-packages/datadog-agent/stable/bin/agent/agent", agentSymlink); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}
	// 4. Install the agent systemd units
	for _, unit := range stableUnits {
		if err = systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}
	for _, unit := range experimentalUnits {
		if err = systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}
	if err = systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}
	// enabling the agentUnit only is enough as others are triggered by it
	if err = systemd.EnableUnit(ctx, agentUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", agentUnit, err)
	}
	_, err = os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	}
	// this is expected during a fresh install with the install script / asible / chef / etc...
	// the config is populated afterwards by the install method and the agent is restarted
	if !os.IsNotExist(err) {
		if err = systemd.StartUnit(ctx, agentUnit); err != nil {
			return err
		}
	}
	return nil
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) error {
	span, ctx := telemetry.StartSpanFromContext(ctx, "remove_agent_units")
	var spanErr error
	defer func() { span.Finish(spanErr) }()
	// stop experiments, they can restart stable agent
	for _, unit := range experimentalUnits {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
			spanErr = err
		}
	}
	// stop stable agents
	for _, unit := range stableUnits {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
			spanErr = err
		}
	}

	if err := systemd.DisableUnit(ctx, agentUnit); err != nil {
		log.Warnf("Failed to disable %s: %s", agentUnit, err)
		spanErr = err
	}

	// remove units from disk
	for _, unit := range experimentalUnits {
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
			spanErr = err
		}
	}
	for _, unit := range stableUnits {
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
			spanErr = err
		}
	}
	if err := os.Remove(agentSymlink); err != nil {
		log.Warnf("Failed to remove agent symlink: %s", err)
		spanErr = err
	}
	installinfo.RemoveInstallInfo()
	return nil
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) error {
	if err := agentPackagePermissions.Ensure("/opt/datadog-packages/datadog-agent/experiment"); err != nil {
		return fmt.Errorf("failed to set package ownerships: %v", err)
	}
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	return systemd.StartUnit(ctx, agentExp, "--no-block")
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment(ctx context.Context) error {
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	return systemd.StartUnit(ctx, agentUnit, "--no-block")
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(ctx context.Context) error {
	// detach from the command context as it will be cancelled by a SIGTERM
	ctx = context.WithoutCancel(ctx)
	return StopAgentExperiment(ctx)
}
