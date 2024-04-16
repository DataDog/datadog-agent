// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
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

// SetupAgentUnits installs and starts the agent units
func SetupAgentUnits(ctx context.Context) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent units: %s, reverting", err)
			RemoveAgentUnits(ctx)
		}
		span.Finish(tracer.WithError(err))
	}()

	if err = setInstallerAgentGroup(ctx); err != nil {
		return
	}

	for _, unit := range stableUnits {
		if err = loadUnit(ctx, unit); err != nil {
			return
		}
	}
	for _, unit := range experimentalUnits {
		if err = loadUnit(ctx, unit); err != nil {
			return
		}
	}

	if err = systemdReload(ctx); err != nil {
		return
	}

	for _, unit := range stableUnits {
		if err = enableUnit(ctx, unit); err != nil {
			return
		}
	}
	// write installinfo before start, or the agent could write it
	if err = installinfo.WriteInstallInfo("updater_package", "manual_update"); err != nil {
		return
	}
	for _, unit := range stableUnits {
		if err = startUnit(ctx, unit); err != nil {
			return
		}
	}
	err = createAgentSymlink(ctx)
	return
}

// RemoveAgentUnits stops and removes the agent units
func RemoveAgentUnits(ctx context.Context) {
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
	// purge experimental units
	for _, unit := range experimentalUnits {
		if err := disableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(ctx, unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	// purge stable units
	for _, unit := range stableUnits {
		if err := disableUnit(ctx, unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(ctx, unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	if err := rmAgentSymlink(ctx); err != nil {
		log.Warnf("Failed to remove agent symlink: %s", err)
	}
	installinfo.RmInstallInfo()
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) error {
	return startUnit(ctx, agentExp)
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment(ctx context.Context) error {
	return startUnit(ctx, agentUnit)
}

// setInstallerAgentGroup adds the dd-installer to the dd-agent group if it's not already in it
func setInstallerAgentGroup(ctx context.Context) error {
	// Get groups of dd-installer
	out, err := exec.Command("id", "-Gn", "dd-installer").Output()
	if err != nil {
		return err
	}
	if strings.Contains(string(out), "dd-agent") {
		return nil
	}
	return executeCommand(ctx, string(addInstallerToAgentGroup))
}
