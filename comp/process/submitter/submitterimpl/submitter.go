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

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	submitterComp "github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	processRunner "github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newSubmitter))
}

// submitter implements the Component.
type submitterImpl struct {
	s *processRunner.CheckSubmitter
}

type dependencies struct {
	fx.In
	Lc  fx.Lifecycle
	Log log.Component

	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	Checks         []types.CheckComponent `group:"check"`
	Forwarders     forwarders.Component
	HostInfo       hostinfo.Component
	Statsd         statsd.ClientInterface
}

type result struct {
	fx.Out

	RTResponseNotifier <-chan types.RTResponse
	Submitter          submitterComp.Component
}

func newSubmitter(deps dependencies) (result, error) {
	s, err := processRunner.NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, deps.HostInfo.Object().HostName, deps.SysProbeConfig)
	if err != nil {
		return result{}, err
	}

	if agent.Enabled(deps.Config, deps.Checks, deps.Log) {
		deps.Lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				return s.Start()
			},
			OnStop: func(context.Context) error {
				s.Stop()
				return nil
			},
		})
	}

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
