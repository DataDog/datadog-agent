// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runnerimpl implements a component to run data collection checks in the Process Agent.
package runnerimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// for testing
var agentEnabled = agent.Enabled

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRunner))
}

// runner implements the Component.
type runnerImpl struct {
	checkRunner    *processRunner.CheckRunner
	providedChecks []types.CheckComponent
}

type dependencies struct {
	fx.In
	Lc  fx.Lifecycle
	Log log.Component

	Submitter   submitter.Component
	EpForwarder eventplatform.Component
	RTNotifier  <-chan types.RTResponse `optional:"true"`

	Checks   []types.CheckComponent `group:"check"`
	HostInfo hostinfo.Component
	SysCfg   sysprobeconfig.Component
	Config   config.Component
}

func newRunner(deps dependencies) (runner.Component, error) {
	checks := fxutil.GetAndFilterGroup(deps.Checks)
	c, err := processRunner.NewRunner(deps.Config, deps.SysCfg.SysProbeObject(), deps.HostInfo.Object(), filterEnabledChecks(checks), deps.RTNotifier)
	if err != nil {
		return nil, err
	}
	c.Submitter = deps.Submitter
	c.EpForwarder = deps.EpForwarder

	runnerComponent := &runnerImpl{
		checkRunner:    c,
		providedChecks: checks,
	}

	if agentEnabled(deps.Config, deps.Checks, deps.Log) {
		deps.Lc.Append(fx.Hook{
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
