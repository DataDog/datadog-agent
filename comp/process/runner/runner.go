// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"context"
	"testing"

	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
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

	Checks   []checks.Check
	HostInfo *checks.HostInfo
	SysCfg   *sysconfig.Config
}

func newRunner(deps dependencies) (Component, error) {
	c, err := r.NewCollector(deps.SysCfg, deps.HostInfo, deps.Checks)
	if err != nil {
		return nil, err
	}

	// TODO: Inject submitter as a component dependency
	c.Submitter, err = r.NewSubmitter(deps.HostInfo.HostName, c.UpdateRTStatus)
	if err != nil {
		return nil, err
	}

	runner := &runner{
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

func (r *runner) GetChecks() []checks.Check {
	// TODO: Change this to use `types.Check` once checks are migrated to components
	return r.collector.GetChecks()
}

func newMock(deps dependencies, t testing.TB) Component {
	return nil
}
