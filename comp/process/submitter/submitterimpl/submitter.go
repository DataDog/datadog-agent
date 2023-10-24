// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package submitterimpl implements a component to submit collected data in the Process Agent to
// supported Datadog intakes.
package submitterimpl

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	submitterComp "github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newSubmitter),
)

// submitter implements the Component.
type submitterImpl struct {
	s *processRunner.CheckSubmitter
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	HostInfo   hostinfo.Component
	Config     config.Component
	Log        log.Component
	Forwarders forwarders.Component
}

type result struct {
	fx.Out

	RTResponseNotifier <-chan types.RTResponse
	Submitter          submitterComp.Component
}

func newSubmitter(deps dependencies) (result, error) {
	s, err := processRunner.NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.HostInfo.Object().HostName)
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
		Submitter: &submitterImpl{
			s: s,
		},
		RTResponseNotifier: s.GetRTNotifierChan(),
	}, nil
}

func (s *submitterImpl) Submit(start time.Time, checkName string, payload *types.Payload) {
	s.s.Submit(start, checkName, payload)
}

func (s *submitterImpl) Start() error {
	return s.s.Start()
}

func (s *submitterImpl) Stop() {
	s.s.Stop()
}
