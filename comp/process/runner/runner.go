// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
)

// runner implements the Component.
type runner struct {
	checks    []types.Check
	submitter submitter.Component
}

type dependencies struct {
	fx.In

	coreConfig     config.Component
	sysProbeConfig sysprobeconfig.Component

	Checks    []types.Check `group:"check"`
	Submitter submitter.Component
}

func newRunner(deps dependencies) (Component, error) {
	return &runner{
		checks:    deps.Checks,
		submitter: deps.Submitter,
	}, nil
}

func (r *runner) Run(ctx context.Context) error {

	for _, c := range r.checks {
		if !c.IsEnabled() {
			continue
		}

		payload, err := c.Run()
		if err != nil {
			return err
		}
		r.submitter.Submit(time.Now(), c.Name(), payload)
	}
	return nil
}

func (r *runner) GetChecks() []types.Check {
	return r.checks
}

func newMock(deps dependencies, t testing.TB) Component {
	// TODO
	return nil
}
