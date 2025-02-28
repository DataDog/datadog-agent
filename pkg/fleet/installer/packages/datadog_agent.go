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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/packagemanager"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	agentPackage      = "datadog-agent"
	agentSymlink      = "/usr/bin/datadog-agent"
	agentUnit         = "datadog-agent.service"
	traceAgentUnit    = "datadog-agent-trace.service"
	processAgentUnit  = "datadog-agent-process.service"
	systemProbeUnit   = "datadog-agent-sysprobe.service"
	securityAgentUnit = "datadog-agent-security.service"
	agentExp          = "datadog-agent-exp.service"
	traceAgentExp     = "datadog-agent-trace-exp.service"
	processAgentExp   = "datadog-agent-process-exp.service"
	systemProbeExp    = "datadog-agent-sysprobe-exp.service"
	securityAgentExp  = "datadog-agent-security-exp.service"
)

var (
	stableUnits = []string{
		agentUnit,
		traceAgentUnit,
		processAgentUnit,
		systemProbeUnit,
		securityAgentUnit,
	}
	experimentalUnits = []string{
		agentExp,
		traceAgentExp,
		processAgentExp,
		systemProbeExp,
		securityAgentExp,
	}
)

var (
	rootOwnedConfigPaths = []string{
		"security-agent.yaml",
		"system-probe.yaml",
		"inject/tracer.yaml",
		"inject",
		"managed",
	}
	// matches omnibus/package-scripts/agent-deb/postinst
	rootOwnedAgentPaths = []string{
		"embedded/bin/system-probe",
		"embedded/bin/security-agent",
		"embedded/share/system-probe/ebpf",
		"embedded/share/system-probe/java",
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
	if err = os.MkdirAll("/etc/datadog-agent", 0755); err != nil {
		return fmt.Errorf("failed to create /etc/datadog-agent: %v", err)
	}
	ddAgentUID, ddAgentGID, err := getAgentIDs()
	if err != nil {
		return fmt.Errorf("error getting dd-agent user and group IDs: %w", err)
	}

	if err = os.Chown("/etc/datadog-agent", ddAgentUID, ddAgentGID); err != nil {
		return fmt.Errorf("failed to chown /etc/datadog-agent: %v", err)
	}
	if err = chownRecursive("/etc/datadog-agent", ddAgentUID, ddAgentGID, rootOwnedConfigPaths); err != nil {
		return fmt.Errorf("failed to chown /etc/datadog-agent: %v", err)
	}
	if err = chownRecursive("/opt/datadog-packages/datadog-agent/stable/", ddAgentUID, ddAgentGID, rootOwnedAgentPaths); err != nil {
		return fmt.Errorf("failed to chown /opt/datadog-packages/datadog-agent/stable/: %v", err)
	}
	// Give root:datadog-agent permissions to system-probe and security-agent config files if they exist
	if err = os.Chown("/etc/datadog-agent/system-probe.yaml", 0, ddAgentGID); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to chown /etc/datadog-agent/system-probe.yaml: %v", err)
	}
	if err = os.Chown("/etc/datadog-agent/security-agent.yaml", 0, ddAgentGID); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to chown /etc/datadog-agent/security-agent.yaml: %v", err)
	}

	if err = systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// enabling the agentUnit only is enough as others are triggered by it
	if err = systemd.EnableUnit(ctx, agentUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", agentUnit, err)
	}
	if err = exec.CommandContext(ctx, "ln", "-sf", "/opt/datadog-packages/datadog-agent/stable/bin/agent/agent", agentSymlink).Run(); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// write installinfo before start, or the agent could write it
	if err = installinfo.WriteInstallInfo("manual_update"); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
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

func chownRecursive(path string, uid int, gid int, ignorePaths []string) error {
	return filepath.WalkDir(path, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		relPath, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		for _, ignore := range ignorePaths {
			if relPath == ignore || strings.HasPrefix(relPath, ignore+string(os.PathSeparator)) {
				return nil
			}
		}
		err = os.Chown(p, uid, gid)
		if err != nil && os.IsNotExist(err) {
			return nil
		}
		return err
	})
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) error {
	ddAgentUID, ddAgentGID, err := getAgentIDs()
	if err != nil {
		return fmt.Errorf("error getting dd-agent user and group IDs: %w", err)
	}
	if err = chownRecursive("/opt/datadog-packages/datadog-agent/experiment/", ddAgentUID, ddAgentGID, rootOwnedAgentPaths); err != nil {
		return fmt.Errorf("failed to chown /opt/datadog-packages/datadog-agent/experiment/: %v", err)
	}
	return systemd.StartUnit(ctx, agentExp, "--no-block")
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment(ctx context.Context) error {
	return systemd.StartUnit(ctx, agentUnit)
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(ctx context.Context) error {
	return StopAgentExperiment(ctx)
}
