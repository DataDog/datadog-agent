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

	"github.com/DataDog/datadog-agent/comp/agent/expvarserver"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/metadatascheduler"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
	"github.com/DataDog/datadog-agent/comp/trace/etwtracer"
	"github.com/DataDog/datadog-agent/comp/trace/etwtracer/etwtracerimpl"

	"github.com/DataDog/datadog-agent/comp/checks/winregistry"
	winregistryimpl "github.com/DataDog/datadog-agent/comp/checks/winregistry/impl"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"

	// checks implemented as components
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect"
	"github.com/DataDog/datadog-agent/comp/checks/agentcrashdetect/agentcrashdetectimpl"
	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog"
	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog/windowseventlogimpl"
	trapserver "github.com/DataDog/datadog-agent/comp/snmptraps/server"
	comptraceconfig "github.com/DataDog/datadog-agent/comp/trace/config"

	// core components
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector"
	processAgent "github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
			_ replay.Component,
			serverDebug dogstatsddebug.Component,
			wmeta workloadmeta.Component,
			taggerComp tagger.Component,
			ac autodiscovery.Component,
			rcclient rcclient.Component,
			forwarder defaultforwarder.Component,
			logsAgent optional.Option[logsAgent.Component],
			processAgent processAgent.Component,
			metadataRunner runner.Component,
			sharedSerializer serializer.MetricSerializer,
			otelcollector otelcollector.Component,
			demultiplexer demultiplexer.Component,
			_ host.Component,
			_ inventoryagent.Component,
			_ inventoryhost.Component,
			_ secrets.Component,
			invChecks inventorychecks.Component,
			_ netflowServer.Component,
			_ trapserver.Component,
			agentAPI internalAPI.Component,
			pkgSigning packagesigning.Component,
			statusComponent status.Component,
			collector collector.Component,
			_ expvarserver.Component,
			metadatascheduler metadatascheduler.Component,
			jmxlogger jmxlogger.Component,
		) error {

			defer StopAgentWithDefaults(agentAPI)

			err := startAgent(
				log,
				flare,
				telemetry,
				sysprobeconfig,
				server,
				serverDebug,
				wmeta,
				taggerComp,
				ac,
				rcclient,
				logsAgent,
				processAgent,
				forwarder,
				sharedSerializer,
				otelcollector,
				demultiplexer,
				agentAPI,
				invChecks,
				statusComponent,
				collector,
				metadatascheduler,
				jmxlogger,
			)
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
				ConfigParams:         config.NewAgentParams(""),
				SecretParams:         secrets.NewEnabledParams(),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
				LogParams:            logimpl.ForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
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

func getPlatformModules() fx.Option {
	return fx.Options(
		agentcrashdetectimpl.Module(),
		etwtracerimpl.Module,
		windowseventlogimpl.Module(),
		winregistryimpl.Module(),
		etwimpl.Module,
		comptraceconfig.Module(),
		fx.Replace(comptraceconfig.Params{
			FailIfAPIKeyMissing: false,
		}),
		// Force the instantiation of the components
		fx.Invoke(func(_ agentcrashdetect.Component) {}),
		fx.Invoke(func(_ etwtracer.Component) {}),
		fx.Invoke(func(_ windowseventlog.Component) {}),
		fx.Invoke(func(_ winregistry.Component) {}),
	)
}
