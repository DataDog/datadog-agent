// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"context"

	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
)

// runner implements the Component.
type runner struct {
	checkRunner *processRunner.CheckRunner
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Submitter  submitter.Component
	RTNotifier <-chan types.RTResponse `optional:"true"`

	Checks   []checks.Check
	HostInfo *checks.HostInfo
	SysCfg   *sysconfig.Config
}

func newRunner(deps dependencies) (Component, error) {
	c, err := processRunner.NewRunner(deps.SysCfg, deps.HostInfo, deps.Checks, deps.RTNotifier)
	if err != nil {
		return nil, err
	}
	c.Submitter = deps.Submitter

	runner := &runner{
		checkRunner: c,
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

// IsRealtimeEnabled
func (r *runner) IsRealtimeEnabled() bool {
	return r.checkRunner.IsRealTimeEnabled()
}

func (r *runner) GetChecks() []checks.Check {
	// TODO: Change this to use `types.Check` once checks are migrated to components
	return r.checkRunner.GetChecks()
}
