// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/submitter"
)

// runner implements the Component.
type runner struct {
	checks    []Check
	submitter submitter.Component
}

type dependencies struct {
	fx.In

	Checks    []Check `group:"check"`
	Submitter submitter.Component
}

func newRunner(deps dependencies) (Component, error) {
	return &runner{
		checks:    deps.Checks,
		submitter: deps.Submitter,
	}, nil
}

func (r *runner) Run(exit <-chan struct{}) error {

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

func (r *runner) GetChecks() []Check {
	return r.checks
}

func newMock(deps dependencies, t testing.TB) Component {
	// TODO
	return nil
}
