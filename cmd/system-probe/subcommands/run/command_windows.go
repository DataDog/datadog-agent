// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	remoteWorkloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-remote"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	localtraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-local"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	eventmonitor "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/fx"
	networktracer "github.com/DataDog/datadog-agent/comp/system-probe/networktracer/fx"
	softwareinventory "github.com/DataDog/datadog-agent/comp/system-probe/softwareinventory/fx"
	traceroute "github.com/DataDog/datadog-agent/comp/system-probe/traceroute/fx"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// StartSystemProbeWithDefaults is a temporary way for windows to use startSystemProbe.
// Starts the agent in the background and then returns.
//
// @ctxChan
//   - After starting the agent the background goroutine waits for a context from
//     this channel, then stops the agent when the context is cancelled.
//
// Returns an error channel that can be used to wait for the agent to stop and get the result.
func StartSystemProbeWithDefaults(ctxChan <-chan context.Context) (<-chan error, error) {
	errChan := make(chan error)

	// run startSystemProbe in the background
	go func() {
		err := runSystemProbe(ctxChan, errChan)
		// notify main routine that this is done, so cleanup can happen
		errChan <- err
	}()

	// Wait for startSystemProbe to complete, or for an error
	err := <-errChan
	if err != nil {
		// startSystemProbe or fx.OneShot failed, caller does not need errChan
		return nil, err
	}

	// startSystemProbe succeeded. provide errChan to caller so they can wait for fxutil.OneShot to stop
	return errChan, nil
}

func runSystemProbe(ctxChan <-chan context.Context, errChan chan error) error {
	return fxutil.OneShot(
		func(
			_ config.Component,
			rcclient rcclient.Component,
			_ healthprobe.Component,
			settings settings.Component,
			deps module.FactoryDependencies,
		) error {
			defer stopSystemProbe()
			err := startSystemProbe(rcclient, settings, deps)
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
				deps.Log.Info("Received stop command, shutting down...")
			case <-signals.ErrorStopper:
				_ = deps.Log.Critical("The Agent has encountered an error, shutting down...")
			case <-ctx.Done():
				deps.Log.Info("Received stop from service manager, shutting down...")
			}

			return nil
		},
		fx.Supply(config.NewAgentParams("")),
		fx.Supply(sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(""))),
		fx.Supply(pidimpl.NewParams("")),
		getSharedFxOption(),
		getPlatformModules(),
	)
}

func getPlatformModules() fx.Option {
	return fx.Options(
		networktracer.Module(),
		softwareinventory.Module(),

		localtraceroute.Module(),
		traceroute.Module(),

		statsd.Module(),
		fx.Provide(func(config config.Component, statsd statsd.Component) (ddgostatsd.ClientInterface, error) {
			return statsd.CreateForHostPort(configutils.GetBindHost(config), config.GetInt("dogstatsd_port"))
		}),
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType: workloadmeta.Remote,
		}),
		ipcfx.ModuleReadWrite(),
		remoteWorkloadfilterfx.Module(),
		remotehostnameimpl.Module(),
		logscompressionfx.Module(),
		remoteTaggerFx.Module(tagger.NewRemoteParams()),
		eventmonitor.Module(),
	)
}
