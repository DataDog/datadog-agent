// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package submitter

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/types"
)

// submitter implements the Component.
type submitter struct {
	s processRunner.Submitter
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	HostInfo *checks.HostInfo
}

func newSubmitter(deps dependencies) (Component, error) {
	s, err := processRunner.NewSubmitter(deps.HostInfo.HostName, nil)
	if err != nil {
		return nil, err
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return s.Start()
		},
		OnStop: func(context.Context) error {
			s.Stop()
			return nil
		},
	})
	return &submitter{
		s: s,
	}, nil
}

func (s *submitter) Submit(start time.Time, checkName string, payload *types.Payload) {
	s.s.Submit(start, checkName, payload)
}

func (s *submitter) Start() error {
	return s.s.Start()
}

func (s *submitter) Stop() {
	s.s.Stop()
}

func newMock(deps dependencies, t testing.TB) Component {
	return nil
}
