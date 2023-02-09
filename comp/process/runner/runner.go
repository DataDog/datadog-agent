// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"context"
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	r "github.com/DataDog/datadog-agent/pkg/process/runner"
)

// runner implements the Component.
type runner struct {
	checks    []types.Check
	submitter submitter.Component

	collector *r.Collector
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	CoreConfig     config.Component
	SysProbeConfig sysprobeconfig.Component

	Checks    []types.Check `group:"check"`
	Submitter submitter.Component
}

func newRunner(deps dependencies) (Component, error) {
	hinfo, err := checks.CollectHostInfo()
	if err != nil {
		return nil, err
	}
	c, err := r.NewCollector(deps.CoreConfig, deps.SysProbeConfig.Object(), hinfo,
		r.GetChecks(deps.SysProbeConfig.Object(), ddconfig.IsAnyContainerFeaturePresent()))
	if err != nil {
		return nil, err
	}

	// TODO: Inject submitter as a component dependency
	c.Submitter, err = r.NewSubmitter(hinfo.HostName, c.UpdateRTStatus)
	if err != nil {
		return nil, err
	}

	runner := &runner{
		checks:    deps.Checks,
		submitter: deps.Submitter,
		collector: c,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: runner.Run,
		OnStop:  runner.Stop,
	})

	return runner, nil
}

func (r *runner) Run(context.Context) error {
	return r.collector.Run()
}

func (r *runner) Stop(context.Context) error {
	r.collector.Stop()
	return nil
}

func (r *runner) GetChecks() []types.Check {
	return r.checks
}

func newMock(deps dependencies, t testing.TB) Component {
	// TODO
	return nil
}
