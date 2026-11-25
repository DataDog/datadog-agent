// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/launchd"
)

// launchdAgentService manages the Agent's launchd jobs for a configuration experiment. It mirrors
// the Linux datadogAgentService's experiment methods (datadog_agent_linux.go), so the darwin
// configuration-experiment hooks read like the Linux ones; where Linux asks systemd to start or
// stop a unit, it asks the unit supervisor (unit_supervisor_darwin.go) for a target.
//
// Keep the method set and the hook bodies in step with Linux:
//   - WriteExperiment  datadog_agent_linux.go WriteExperiment
//   - StartExperiment  datadog_agent_linux.go StartExperiment
//   - StopExperiment   datadog_agent_linux.go StopExperiment
//   - RemoveExperiment datadog_agent_linux.go RemoveExperiment
//   - RestartStable    datadog_agent_linux.go RestartStable
type launchdAgentService struct{}

// agentService is the darwin counterpart of the Linux agentService.
var agentService = launchdAgentService{}

// WriteExperiment writes the experiment job definitions, the installer daemon's included.
func (launchdAgentService) WriteExperiment(_ HookContext) error {
	if err := configExperimentJobSet().Write(launchd.Experiment); err != nil {
		return fmt.Errorf("could not write the experiment job definitions: %w", err)
	}
	return nil
}

// StartExperiment starts the experiment jobs, which stops the stable ones. A start that fails asks
// the supervisor for the stable jobs instead (withdrawUnitsRequest): the hook then discards the
// experiment configuration, and a supervisor still working on the start must not go on with it.
func (launchdAgentService) StartExperiment(ctx HookContext) error {
	err := requestUnits(ctx, unitsTargetExperiment)
	if err != nil {
		withdrawUnitsRequest(ctx)
	}
	return err
}

// StopExperiment stops the experiment jobs, which starts the stable ones again. Stable jobs that
// already run are left alone.
func (launchdAgentService) StopExperiment(ctx HookContext) error {
	return requestUnits(ctx, unitsTargetStable)
}

// RemoveExperiment removes the Agent's experiment job definitions. The installer daemon's is
// removed by the unit supervisor once it has replaced that daemon.
func (launchdAgentService) RemoveExperiment(_ HookContext) error {
	return agentJobSet().Remove(launchd.Experiment)
}

// RestartStable restarts the stable jobs, so they read the stable configuration -- the promoted
// one, after a promote -- and stops the experiment jobs.
func (launchdAgentService) RestartStable(ctx HookContext) error {
	return requestUnits(ctx, unitsTargetRestartStable)
}
