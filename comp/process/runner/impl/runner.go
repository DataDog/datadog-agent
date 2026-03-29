// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runnerimpl implements a component to run data collection checks in the Process Agent.
package runnerimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/def"
	runner "github.com/DataDog/datadog-agent/comp/process/runner/def"
	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
)

// for testing
var agentEnabled = agent.Enabled

// runnerImpl implements the Component.
type runnerImpl struct {
	checkRunner    *processRunner.CheckRunner
	providedChecks []types.CheckComponent
}

type dependencies struct {
	compdef.In
	Lc  compdef.Lifecycle
	Log log.Component

	Submitter  submitter.Component
	RTNotifier <-chan types.RTResponse `optional:"true"`

	Checks   []types.CheckComponent `group:"check"`
	HostInfo hostinfo.Component
	SysCfg   sysprobeconfig.Component
	Config   config.Component
	Tagger   tagger.Component
}

// NewComponent creates a new runner component.
func NewComponent(deps dependencies) (runner.Component, error) {
	checks := filterNilChecks(deps.Checks)
	c, err := processRunner.NewRunner(deps.Config, deps.SysCfg.SysProbeObject(), deps.HostInfo.Object(), filterEnabledChecks(checks), deps.RTNotifier)
	if err != nil {
		return nil, err
	}
	c.Submitter = deps.Submitter

	runnerComponent := &runnerImpl{
		checkRunner:    c,
		providedChecks: checks,
	}

	if agentEnabled(deps.Config, deps.Checks, deps.Log) {
		deps.Lc.Append(compdef.Hook{
			OnStart: runnerComponent.Run,
			OnStop:  runnerComponent.stop,
		})
	}

	return runnerComponent, nil
}

func (r *runnerImpl) Run(context.Context) error {
	return r.checkRunner.Run()
}

func (r *runnerImpl) stop(context.Context) error {
	r.checkRunner.Stop()
	return nil
}

// filterNilChecks removes nil values from an fx group of CheckComponent.
func filterNilChecks(group []types.CheckComponent) []types.CheckComponent {
	result := make([]types.CheckComponent, 0, len(group))
	for _, item := range group {
		if item != nil {
			result = append(result, item)
		}
	}
	return result
}

func filterEnabledChecks(providedChecks []types.CheckComponent) []checks.Check {
	enabledChecks := make([]checks.Check, 0, len(providedChecks))
	for _, check := range providedChecks {
		if check.Object().IsEnabled() {
			enabledChecks = append(enabledChecks, check.Object())
		}
	}
	return enabledChecks
}

// IsRealtimeEnabled checks the runner to see if it is running the process check in realtime mode.
// This is primarily used in tests.
func (r *runnerImpl) IsRealtimeEnabled() bool {
	return r.checkRunner.IsRealTimeEnabled()
}

// GetChecks returns the checks that are currently enabled and provided to the runner
func (r *runnerImpl) GetChecks() []checks.Check {
	return r.checkRunner.GetChecks()
}

// GetProvidedChecks returns all provided checks, enabled or not.
func (r *runnerImpl) GetProvidedChecks() []types.CheckComponent {
	return r.providedChecks
}
