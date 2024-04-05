// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
func SetupAgentUnits() (err error) {
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent units: %s, reverting", err)
			RemoveAgentUnits()
		}
	}()

	for _, unit := range stableUnits {
		if err = loadUnit(unit); err != nil {
			return
		}
	}
	for _, unit := range experimentalUnits {
		if err = loadUnit(unit); err != nil {
			return
		}
	}

	if err = systemdReload(); err != nil {
		return
	}

	for _, unit := range stableUnits {
		if err = enableUnit(unit); err != nil {
			return
		}
	}
	for _, unit := range stableUnits {
		if err = startUnit(unit); err != nil {
			return
		}
	}
	if err = createAgentSymlink(); err != nil {
		return
	}
	err = installinfo.WriteInstallInfo("updater_package", "manual_update_via_apt")
	return
}

// RemoveAgentUnits stops and removes the agent units
func RemoveAgentUnits() {
	// stop experiments, they can restart stable agent
	for _, unit := range experimentalUnits {
		if err := stopUnit(unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
	// stop stable agents
	for _, unit := range stableUnits {
		if err := stopUnit(unit); err != nil {
			log.Warnf("Failed to stop %s: %s", unit, err)
		}
	}
	// purge experimental units
	for _, unit := range experimentalUnits {
		if err := disableUnit(unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	// purge stable units
	for _, unit := range stableUnits {
		if err := disableUnit(unit); err != nil {
			log.Warnf("Failed to disable %s: %s", unit, err)
		}
		if err := removeUnit(unit); err != nil {
			log.Warnf("Failed to remove %s: %s", unit, err)
		}
	}
	if err := rmAgentSymlink(); err != nil {
		log.Warnf("Failed to remove agent symlink: %s", err)
	}
	installinfo.RmInstallInfo()
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment() error {
	return startUnit(agentExp)
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment() error {
	return startUnit(agentUnit)
}
