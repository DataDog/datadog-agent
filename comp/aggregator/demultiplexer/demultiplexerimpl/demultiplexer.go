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
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newDemultiplexer),
		fx.Supply(params))
}

type dependencies struct {
	fx.In
	Lc                     fx.Lifecycle
	Config                 config.Component
	Log                    log.Component
	SharedForwarder        defaultforwarder.Component
	OrchestratorForwarder  orchestratorforwarder.Component
	EventPlatformForwarder eventplatform.Component
	HaAgent                haagent.Component
	Compressor             compression.Component
	Tagger                 tagger.Component
	Hostname               hostnameinterface.Component
	FilterList             filterlist.Component

	Params Params
}

type demultiplexer struct {
	*aggregator.AgentDemultiplexer
}

type provides struct {
	fx.Out
	Comp demultiplexerComp.Component

	SenderManager           sender.SenderManager
	StatusProvider          status.InformationProvider
	AggregatorDemultiplexer aggregator.Demultiplexer
}

func newDemultiplexer(deps dependencies) (provides, error) {
	hostnameDetected, err := deps.Hostname.Get(context.TODO())
	if err != nil {
		if deps.Params.continueOnMissingHostname {
			deps.Log.Warnf("Error getting hostname: %s", err)
			hostnameDetected = ""
		} else {
			return provides{}, deps.Log.Errorf("Error while getting hostname, exiting: %v", err)
		}
	}
	options := createAgentDemultiplexerOptions(deps.Config, deps.Params)
	agentDemultiplexer := aggregator.InitAndStartAgentDemultiplexer(
		deps.Log,
		deps.SharedForwarder,
		deps.OrchestratorForwarder,
		options,
		deps.EventPlatformForwarder,
		deps.HaAgent,
		deps.Compressor,
		deps.Tagger,
		deps.FilterList,
		hostnameDetected,
	)
	demultiplexer := demultiplexer{
		AgentDemultiplexer: agentDemultiplexer,
	}
	deps.Lc.Append(fx.Hook{OnStop: func(_ context.Context) error {
		agentDemultiplexer.Stop(true)
		return nil
	}})

	return provides{
		Comp:          demultiplexer,
		SenderManager: demultiplexer,
		StatusProvider: status.NewInformationProvider(demultiplexerStatus{
			Log: deps.Log,
		}),
		AggregatorDemultiplexer: demultiplexer,
	}, nil
}

func createAgentDemultiplexerOptions(config config.Component, params Params) aggregator.AgentDemultiplexerOptions {
	options := aggregator.DefaultAgentDemultiplexerOptions()
	if params.useDogstatsdNoAggregationPipelineConfig {
		options.EnableNoAggregationPipeline = config.GetBool("dogstatsd_no_aggregation_pipeline")
	}

	// Override FlushInterval only if flushInterval is set by the user
	if v, ok := params.flushInterval.Get(); ok {
		options.FlushInterval = v
	}
	return options
}
