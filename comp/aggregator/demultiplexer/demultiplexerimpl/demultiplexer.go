// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package demultiplexerimpl defines the aggregator demultiplexer
package demultiplexerimpl

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path"

	"go.uber.org/fx"

	demultiplexerComp "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/zstd"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newDemultiplexer))
}

type dependencies struct {
	fx.In
	Lc                     fx.Lifecycle
	Log                    log.Component
	SharedForwarder        defaultforwarder.Component
	OrchestratorForwarder  orchestratorforwarder.Component
	EventPlatformForwarder eventplatform.Component
	Compressor             compression.Component
	Config                 config.Component

	Params Params
}

type demultiplexer struct {
	*aggregator.AgentDemultiplexer
}

type demultiplexerEndpoint struct {
	Comp   demultiplexerComp.Component
	Config config.Component
	Log    log.Component
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
	DiagnosticSenderManager diagnosesendermanager.Component
	SenderManager           sender.SenderManager
	StatusProvider          status.InformationProvider
	Endpoint                api.AgentEndpointProvider
	AggregatorDemultiplexer aggregator.Demultiplexer
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
		deps.EventPlatformForwarder,
		deps.Compressor,
		hostnameDetected)
	demultiplexer := demultiplexer{
		AgentDemultiplexer: agentDemultiplexer,
	}
	deps.Lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
		agentDemultiplexer.Stop(true)
		return nil
	}})

	endpoint := demultiplexerEndpoint{
		Comp:   demultiplexer,
		Config: deps.Config,
		Log:    deps.Log,
	}

	return provides{
		Comp:                    demultiplexer,
		DiagnosticSenderManager: demultiplexer,
		SenderManager:           demultiplexer,
		StatusProvider: status.NewInformationProvider(demultiplexerStatus{
			Log: deps.Log,
		}),
		Endpoint:                api.NewAgentEndpointProvider(endpoint.dumpDogstatsdContexts, "/dogstatsd-contexts-dump", "POST"),
		AggregatorDemultiplexer: demultiplexer,
	}, nil
}

// LazyGetSenderManager gets an instance of SenderManager lazily.
func (demux demultiplexer) LazyGetSenderManager() (sender.SenderManager, error) {
	return demux, nil
}

func (demuxendpoint demultiplexerEndpoint) dumpDogstatsdContexts(w http.ResponseWriter, _ *http.Request) {
	path, err := demuxendpoint.dumpDogstatsdContextsImpl()
	if err != nil {
		utils.SetJSONError(w, demuxendpoint.Log.Errorf("Failed to create dogstatsd contexts dump: %v", err), 500)
		return
	}

	resp, err := json.Marshal(path)
	if err != nil {
		utils.SetJSONError(w, demuxendpoint.Log.Errorf("Failed to serialize response: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func (demuxendpoint demultiplexerEndpoint) dumpDogstatsdContextsImpl() (string, error) {
	path := path.Join(demuxendpoint.Config.GetString("run_path"), "dogstatsd_contexts.json.zstd")

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}

	c := zstd.NewWriter(f)

	w := bufio.NewWriter(c)

	for _, err := range []error{demuxendpoint.Comp.DumpDogstatsdContexts(w), w.Flush(), c.Close(), f.Close()} {
		if err != nil {
			return "", err
		}
	}

	return path, nil
}
