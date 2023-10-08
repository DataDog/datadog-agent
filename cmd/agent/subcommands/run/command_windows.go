// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package run implements 'agent run' (and deprecated 'agent start').
package run

import (
	"context"
	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"

	// checks implemented as components
	"github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect"

	// core components
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsdDebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	// runtime init routines
)

// StartAgentWithDefaults is a temporary way for the windows service to use startAgent.
// Starts the agent in the background and then returns.
//
// @ctxChan
//   - After starting the agent the background goroutine waits for a context from
//     this channel, then stops the agent when the context is cancelled.
//
// Returns an error channel that can be used to wait for the agent to stop and get the result.
func StartAgentWithDefaults(ctxChan <-chan context.Context) (<-chan error, error) {
	errChan := make(chan error)

	// run startAgent in an app, so that the log and config components get initialized
	go func() {
		err := fxutil.OneShot(func(
			log log.Component,
			config config.Component,
			flare flare.Component,
			telemetry telemetry.Component,
			sysprobeconfig sysprobeconfig.Component,
			server dogstatsdServer.Component,
			serverDebug dogstatsdDebug.Component,
			capture replay.Component,
			rcclient rcclient.Component,
			forwarder defaultforwarder.Component,
			logsAgent util.Optional[logsAgent.Component],
			metadataRunner runner.Component,
			sharedSerializer serializer.MetricSerializer,
			otelcollector otelcollector.Component,
			_ netflowServer.Component,
			_ agentcrashdetect.Component,
			_ comptraceconfig.Component,
		) error {

			defer StopAgentWithDefaults(server)

			err := startAgent(&cliParams{GlobalParams: &command.GlobalParams{}}, log, flare, telemetry, sysprobeconfig, server, capture, serverDebug, rcclient, logsAgent, forwarder, sharedSerializer, otelcollector)
			if err != nil {
				return err
			}

			// notify outer that startAgent finished
			errChan <- err
			// wait for context
			ctx := <-ctxChan

			// Wait for stop signal
			select {
			case <-signals.Stopper:
				log.Info("Received stop command, shutting down...")
			case <-signals.ErrorStopper:
				_ = log.Critical("The Agent has encountered an error, shutting down...")
			case <-ctx.Done():
				log.Info("Received stop from service manager, shutting down...")
			}

			return nil
		},
			// no config file path specification in this situation
			fx.Supply(core.BundleParams{
				ConfigParams:         config.NewAgentParamsWithSecrets(""),
				SysprobeConfigParams: sysprobeconfig.NewParams(),
				LogParams:            log.LogForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
			}),
			getSharedFxOption(),
			getPlatformModules(),
		)
		// notify caller that fx.OneShot is done
		errChan <- err
	}()

	// Wait for startAgent to complete, or for an error
	err := <-errChan
	if err != nil {
		// startAgent or fx.OneShot failed, caller does not need errChan
		return nil, err
	}

	// startAgent succeeded. provide errChan to caller so they can wait for fxutil.OneShot to stop
	return errChan, nil
}

func run(log log.Component,
	config config.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsdDebug.Component,
	forwarder defaultforwarder.Component,
	rcclient rcclient.Component,
	metadataRunner runner.Component,
	demux *aggregator.AgentDemultiplexer,
	sharedSerializer serializer.MetricSerializer,
	cliParams *cliParams,
	logsAgent util.Optional[logsAgent.Component],
	otelcollector otelcollector.Component,
	_ netflowServer.Component,
	_ agentcrashdetect.Component,
	_ comptraceconfig.Component,
) error {
	// commonRun provides a mechanism to have the shared run function not require the unused components
	// (i.e. here `_ netflowServer`, `_ agentcrashdetect`, etc.).  The run function can have different
	// parameters on different platforms based on platform-specific components.  commonRun is the shared initialization.

	return commonRun(log, config, flare, telemetry, sysprobeconfig, server, capture, serverDebug, forwarder, rcclient, metadataRunner, demux, sharedSerializer, cliParams, logsAgent, otelcollector)
}

func getPlatformModules() fx.Option {
	return fx.Options(
		agentcrashdetect.Module,
		comptraceconfig.Module,
		fx.Replace(comptraceconfig.Params{
			FailIfAPIKeyMissing: false,
		}),
	)
}
