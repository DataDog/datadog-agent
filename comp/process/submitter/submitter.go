// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package submitter

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
)

// submitter implements the Component.
type submitter struct {
	s *processRunner.CheckSubmitter
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	HostInfo *checks.HostInfo
}

type result struct {
	fx.Out

	RTResponseNotifier <-chan types.RTResponse
	Submitter          Component
}

func newSubmitter(deps dependencies) (result, error) {
	s, err := processRunner.NewSubmitter(deps.HostInfo.HostName)
	if err != nil {
		return result{}, err
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
	return result{
		Submitter: &submitter{
			s: s,
		},
		RTResponseNotifier: s.GetRTNotifierChan(),
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
