// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerimpl defines the aggregator demultiplexer
package demultiplexerimpl

import (
	"context"

	"go.uber.org/fx"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newDemultiplexer))
}

type dependencies struct {
	fx.In
	Lc                    fx.Lifecycle
	Log                   log.Component
	SharedForwarder       defaultforwarder.Component
	OrchestratorForwarder orchestratorforwarder.Component

	Params Params
}

type demultiplexer struct {
	*aggregator.AgentDemultiplexer
}

type provides struct {
	fx.Out
	Comp demultiplexerComp.Component

	// Both demultiplexer.Component and diagnosesendermanager.Component expose a different instance of SenderManager.
	// It means that diagnosesendermanager.Component must not be used when there is demultiplexer.Component instance.
	//
	// newDemultiplexer returns both demultiplexer.Component and diagnosesendermanager.Component (Note: demultiplexer.Component
	// implements diagnosesendermanager.Component). This has the nice consequence of preventing having
	// demultiplexerimpl.Module and diagnosesendermanagerimpl.Module in the same fx.App because there would
	// be two ways to create diagnosesendermanager.Component.
	SenderManager  diagnosesendermanager.Component
	StatusProvider status.InformationProvider
}

func newDemultiplexer(deps dependencies) (provides, error) {
	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		if deps.Params.ContinueOnMissingHostname {
			deps.Log.Warnf("Error getting hostname: %s", err)
			hostnameDetected = ""
		} else {
			return provides{}, deps.Log.Errorf("Error while getting hostname, exiting: %v", err)
		}
	}

	agentDemultiplexer := aggregator.InitAndStartAgentDemultiplexer(
		deps.Log,
		deps.SharedForwarder,
		deps.OrchestratorForwarder,
		deps.Params.AgentDemultiplexerOptions,
		hostnameDetected)
	demultiplexer := demultiplexer{
		AgentDemultiplexer: agentDemultiplexer,
	}
	deps.Lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		agentDemultiplexer.Stop(true)
		return nil
	}})

	return provides{
		Comp:          demultiplexer,
		SenderManager: demultiplexer,
		StatusProvider: status.NewInformationProvider(demultiplexerStatus{
			Log: deps.Log,
		}),
	}, nil
}

// LazyGetSenderManager gets an instance of SenderManager lazily.
func (demux demultiplexer) LazyGetSenderManager() (sender.SenderManager, error) {
	return demux, nil
}
