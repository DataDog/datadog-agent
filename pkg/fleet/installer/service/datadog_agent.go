// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	pathOldAgent      = "/opt/datadog-agent"
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

// SetupAgent installs and starts the agent
func SetupAgent(ctx context.Context, _ []string) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent: %s, reverting", err)
			err = errors.Join(err, RemoveAgent(ctx))
		}
		span.Finish(tracer.WithError(err))
	}()

	if err = stopOldAgentUnits(ctx); err != nil {
		return err
	}

	for _, unit := range stableUnits {
		if err = loadUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}
	for _, unit := range experimentalUnits {
		if err = loadUnit(ctx, unit); err != nil {
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

	if err = systemdReload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// enabling the agentUnit only is enough as others are triggered by it
	if err = enableUnit(ctx, agentUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", agentUnit, err)
	}
	if err = exec.CommandContext(ctx, "ln", "-sf", "/opt/datadog-packages/datadog-agent/stable/bin/agent/agent", agentSymlink).Run(); err != nil {
		return fmt.Errorf("failed to create symlink: %v", err)
	}

	// write installinfo before start, or the agent could write it
	// TODO: add installer version properly
	if err = installinfo.WriteInstallInfo("installer_package", "manual_update"); err != nil {
		return fmt.Errorf("failed to write install info: %v", err)
	}

	_, err = os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	}
	// this is expected during a fresh install with the install script / asible / chef / etc...
	// the config is populated afterwards by the install method and the agent is restarted
	if !os.IsNotExist(err) {
		if err = startUnit(ctx, agentUnit); err != nil {
			return err
		}
	}
	if err = removeOldAgentFiles(); err != nil {
		return fmt.Errorf("failed to remove old agent files: %v", err)
	}
	return nil
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) error {
	span, ctx := tracer.StartSpanFromContext(ctx, "remove_agent_units")
	defer span.Finish()
	// stop experiments, they can restart stable agent
	for _, unit := range experimentalUnits {
		if err := stopUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
	// stop stable agents
	for _, unit := range stableUnits {
		if err := stopUnit(ctx, unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}

	if err := disableUnit(ctx, agentUnit); err != nil {
		log.Warnf("Failed to disable %s: %s", agentUnit, err)
	}

	// remove units from disk
	for _, unit := range experimentalUnits {
		if err := removeUnit(ctx, unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	for _, unit := range stableUnits {
		if err := removeUnit(ctx, unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	if err := os.Remove(agentSymlink); err != nil {
		log.Warnf("Failed to remove agent symlink: %s", err)
	}
	installinfo.RmInstallInfo()
	// TODO: Return error to caller?
	return nil
}

func oldAgentInstalled() bool {
	_, err := os.Stat(pathOldAgent)
	return err == nil
}

func stopOldAgentUnits(ctx context.Context) error {
	if !oldAgentInstalled() {
		return nil
	}
	span, ctx := tracer.StartSpanFromContext(ctx, "remove_old_agent_units")
	defer span.Finish()
	for _, unit := range stableUnits {
		if err := stopUnit(ctx, unit); err != nil {
			exitError, ok := err.(*exec.ExitError)
			if ok && exitError.ExitCode() == 5 {
				// exit code 5 means the unit is not loaded, we can continue
				continue
			}
			return fmt.Errorf("failed to stop %s: %v", unit, err)
		}
		if err := disableUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to disable %s: %v", unit, err)
		}
	}
	return nil
}

// removeOldAgentFiles removes old agent files
func removeOldAgentFiles() error {
	if !oldAgentInstalled() {
		return nil
	}
	return os.RemoveAll(pathOldAgent)
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) error {
	return startUnit(ctx, agentExp, "--no-block")
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment(ctx context.Context) error {
	return startUnit(ctx, agentUnit)
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(ctx context.Context) error {
	return StopAgentExperiment(ctx)
}
