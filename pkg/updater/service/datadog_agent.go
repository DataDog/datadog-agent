// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

// Package service provides a way to interact with os services
package service

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
func SetupAgentUnits() error {
	runner, err := newScriptRunner()
	if err != nil {
		return err
	}
	defer runner.close()
	for _, unit := range stableUnits {
		if err := runner.loadUnit(unit); err != nil {
			return err
		}
	}
	for _, unit := range experimentalUnits {
		if err := runner.loadUnit(unit); err != nil {
			return err
		}
	}

	if err = runner.systemdReload(); err != nil {
		return err
	}

	for _, unit := range stableUnits {
		if err := runner.enableUnit(unit); err != nil {
			return err
		}
	}
	for _, unit := range stableUnits {
		if err := runner.startUnit(unit); err != nil {
			return err
		}
	}
	return nil
}

// RemoveAgentUnits stops and removes the agent units
func RemoveAgentUnits() error {
	runner, err := newScriptRunner()
	if err != nil {
		return err
	}
	defer runner.close()
	// stop experiments, they can restart stable agent
	for _, unit := range experimentalUnits {
		if err := runner.stopUnit(unit); err != nil {
			return err
		}
	}
	// stop stable agents
	for _, unit := range stableUnits {
		if err := runner.stopUnit(unit); err != nil {
			return err
		}
	}
	// purge experimental units
	for _, unit := range experimentalUnits {
		if err := runner.disableUnit(unit); err != nil {
			return err
		}
		if err := runner.removeUnit(unit); err != nil {
			return err
		}
	}
	// purge stable units
	for _, unit := range stableUnits {
		if err := runner.disableUnit(unit); err != nil {
			return err
		}
		if err := runner.removeUnit(unit); err != nil {
			return err
		}
	}
	return nil
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment() error {
	runner, err := newScriptRunner()
	if err != nil {
		return err
	}
	defer runner.close()

	return runner.startUnit(agentExp)
}

// StopAgentExperiment stops the agent experiment
func StopAgentExperiment() error {
	runner, err := newScriptRunner()
	if err != nil {
		return err
	}
	defer runner.close()

	return runner.startUnit(agentUnit)
}
