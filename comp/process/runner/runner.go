// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
)

// runner implements the Component.
type runner struct {
	checkRunner    *processRunner.CheckRunner
	providedChecks []types.CheckComponent
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Submitter  submitter.Component
	RTNotifier <-chan types.RTResponse `optional:"true"`

	Checks   []types.CheckComponent `group:"check"`
	HostInfo hostinfo.Component
	SysCfg   sysprobeconfig.Component
	Config   config.Component
}

func newRunner(deps dependencies) (Component, error) {
	c, err := processRunner.NewRunner(deps.Config, deps.SysCfg.Object(), deps.HostInfo.Object(), filterEnabledChecks(deps.Checks), deps.RTNotifier)
	if err != nil {
		return nil, err
	}
	c.Submitter = deps.Submitter

	runner := &runner{
		checkRunner:    c,
		providedChecks: deps.Checks,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: runner.Run,
		OnStop:  runner.Stop,
	})

	return runner, nil
}

func (r *runner) Run(context.Context) error {
	return r.checkRunner.Run()
}

func (r *runner) Stop(context.Context) error {
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
func (r *runner) IsRealtimeEnabled() bool {
	return r.checkRunner.IsRealTimeEnabled()
}

// GetChecks returns the checks that are currently enabled and provided to the runner
func (r *runner) GetChecks() []checks.Check {
	return r.checkRunner.GetChecks()
}

// GetProvidedChecks returns all provided checks, enabled or not.
func (r *runner) GetProvidedChecks() []types.CheckComponent {
	return r.providedChecks
}
