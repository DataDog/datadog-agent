// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package main implements main
package main

import (
	"context"
	"os"
	"path"

	"go.uber.org/fx"

	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	saconfig "github.com/DataDog/datadog-agent/cmd/security-agent/config"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/runtime"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/start"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

type service struct {
	servicemain.DefaultSettings
}

var (
	defaultSecurityAgentConfigFilePaths = []string{
		path.Join(commonpath.DefaultConfPath, "datadog.yaml"),
		path.Join(commonpath.DefaultConfPath, "security-agent.yaml"),
	}
	defaultSysProbeConfPath = path.Join(commonpath.DefaultConfPath, "system-probe.yaml")
)

// Name returns the service name
func (s *service) Name() string {
	return saconfig.ServiceName
}

// Init does nothing for now.
func (s *service) Init() error {
	return nil
}

type cliParams struct {
	*command.GlobalParams
}

// Run actually runs the service; blocks until service exits.
func (s *service) Run(svcctx context.Context) error {

	params := &cliParams{}
	err := fxutil.OneShot(
		func(log log.Component, config config.Component, _ secrets.Component, statsd statsd.Component, sysprobeconfig sysprobeconfig.Component,
			telemetry telemetry.Component, _ workloadmeta.Component, params *cliParams, demultiplexer demultiplexer.Component, statusComponent status.Component) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer start.StopAgent(cancel, log)

			err := start.RunAgent(ctx, log, config, telemetry, demultiplexer, statusComponent)
			if err != nil {
				return err
			}

			// Wait for stop signal
			<-svcctx.Done()
			log.Info("Received stop from service manager, shutting down...")

			return nil
		},
		fx.Supply(params),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewSecurityAgentParams(defaultSecurityAgentConfigFilePaths),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(defaultSysProbeConfPath)),
			LogParams:            logimpl.ForDaemon(command.LoggerName, "security_agent.log_file", setup.DefaultSecurityAgentLogFile),
		}),
		core.Bundle(),
		dogstatsd.ClientBundle,
		forwarder.Bundle(),
		fx.Provide(defaultforwarder.NewParamsWithResolvers),
		compressionimpl.Module(),
		demultiplexerimpl.Module(),
		orchestratorForwarderImpl.Module(),
		fx.Supply(orchestratorForwarderImpl.NewDisabledParams()),
		eventplatformimpl.Module(),
		fx.Supply(eventplatformimpl.NewDisabledParams()),
		eventplatformreceiverimpl.Module(),
		fx.Supply(demultiplexerimpl.NewDefaultParams()),

		// workloadmeta setup
		collectors.GetCatalog(),
		workloadmeta.Module(),
		fx.Provide(func(config config.Component) workloadmeta.Params {

			catalog := workloadmeta.NodeAgent

			if config.GetBool("security_agent.remote_workloadmeta") {
				catalog = workloadmeta.Remote
			}

			return workloadmeta.Params{
				AgentType: catalog,
			}
		}),
		fx.Provide(func(log log.Component, config config.Component, statsd statsd.Component, demultiplexer demultiplexer.Component, wmeta workloadmeta.Component) (status.InformationProvider, *agent.RuntimeSecurityAgent, error) {
			stopper := startstop.NewSerialStopper()

			statsdClient, err := statsd.CreateForHostPort(setup.GetBindHost(config), config.GetInt("dogstatsd_port"))

			if err != nil {
				return status.NewInformationProvider(nil), nil, err
			}

			hostnameDetected, err := utils.GetHostnameWithContextAndFallback(context.TODO())
			if err != nil {
				return status.NewInformationProvider(nil), nil, err
			}

			runtimeAgent, err := runtime.StartRuntimeSecurity(log, config, hostnameDetected, stopper, statsdClient, demultiplexer, wmeta)
			if err != nil {
				return status.NewInformationProvider(nil), nil, err
			}

			if runtimeAgent == nil {
				return status.NewInformationProvider(nil), nil, nil
			}

			// TODO - components: Do not remove runtimeAgent ref until "github.com/DataDog/datadog-agent/pkg/security/agent" is a component so they're not GCed
			return status.NewInformationProvider(runtimeAgent.StatusProvider()), runtimeAgent, nil
		}),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: python.GetPythonVersion,
			},
		),
		fx.Provide(func(config config.Component) status.HeaderInformationProvider {
			return status.NewHeaderInformationProvider(hostimpl.StatusProvider{
				Config: config,
			})
		}),

		statusimpl.Module(),

		fetchonlyimpl.Module(),
		configsyncimpl.OptionalModule(),
		// Force the instantiation of the component
		fx.Invoke(func(_ optional.Option[configsync.Component]) {}),
	)

	return err
}

func main() {
	// if command line arguments are supplied, even in a non-interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		if servicemain.RunningAsWindowsService() {
			servicemain.Run(&service{})
			return
		}
	}

	rootCmd := command.MakeCommand(subcommands.SecurityAgentSubcommands())
	os.Exit(runcmd.Run(rootCmd))
}
