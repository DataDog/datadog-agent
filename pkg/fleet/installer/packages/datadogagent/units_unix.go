// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package datadogagent

import (
	"context"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/systemd"
)

const (
	agentUnit             = "datadog-agent.service"
	installerAgentUnit    = "datadog-agent-installer.service"
	traceAgentUnit        = "datadog-agent-trace.service"
	processAgentUnit      = "datadog-agent-process.service"
	systemProbeUnit       = "datadog-agent-sysprobe.service"
	securityAgentUnit     = "datadog-agent-security.service"
	agentExpUnit          = "datadog-agent-exp.service"
	installerAgentExpUnit = "datadog-agent-installer-exp.service"
	traceAgentExpUnit     = "datadog-agent-trace-exp.service"
	processAgentExpUnit   = "datadog-agent-process-exp.service"
	systemProbeExpUnit    = "datadog-agent-sysprobe-exp.service"
	securityAgentExpUnit  = "datadog-agent-security-exp.service"
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
	expUnits = []string{
		agentExpUnit,
		installerAgentExpUnit,
		traceAgentExpUnit,
		processAgentExpUnit,
		systemProbeExpUnit,
		securityAgentExpUnit,
	}
)

func stopAndRemoveAgentUnits(ctx context.Context, experiment bool, mainUnit string) error {
	units, err := systemd.ListOnDiskAgentUnits(experiment)
	if err != nil {
		return fmt.Errorf("failed to list agent units: %v", err)
	}

	for _, unit := range units {
		if err := systemd.StopUnit(ctx, unit); err != nil {
			return err
		}
	}

	if err := systemd.DisableUnit(ctx, mainUnit); err != nil {
		return err
	}

	for _, unit := range units {
		if err := systemd.RemoveUnit(ctx, unit); err != nil {
			return err
		}
	}
	return nil
}

func setupAndStartAgentUnits(ctx context.Context, units []string, mainUnit string) error {
	for _, unit := range units {
		if err := systemd.WriteEmbeddedUnit(ctx, unit); err != nil {
			return fmt.Errorf("failed to load %s: %v", unit, err)
		}
	}

	if err := systemd.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	// enabling the core agent unit only is enough as others are triggered by it
	if err := systemd.EnableUnit(ctx, mainUnit); err != nil {
		return fmt.Errorf("failed to enable %s: %v", mainUnit, err)
	}

	_, err := os.Stat("/etc/datadog-agent/datadog.yaml")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if /etc/datadog-agent/datadog.yaml exists: %v", err)
	} else if os.IsNotExist(err) {
		// this is expected during a fresh install with the install script / ansible / chef / etc...
		// the config is populated afterwards by the install method and the agent is restarted
		return nil
	}
	if err = systemd.StartUnit(ctx, mainUnit); err != nil {
		return err
	}
	return nil
}
