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
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/start"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
			telemetry telemetry.Component, _ workloadmeta.Component, params *cliParams, demultiplexer demultiplexer.Component) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer start.StopAgent(cancel, log)

			err := start.RunAgent(ctx, log, config, statsd, sysprobeconfig, telemetry, "", demultiplexer)
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
			LogParams:            logimpl.ForDaemon(command.LoggerName, "security_agent.log_file", pkgconfig.DefaultSecurityAgentLogFile),
		}),
		core.Bundle(),
		dogstatsd.ClientBundle,
		forwarder.Bundle(),
		fx.Provide(defaultforwarder.NewParamsWithResolvers),
		demultiplexerimpl.Module(),
		orchestratorForwarderImpl.Module(),
		fx.Supply(orchestratorForwarderImpl.NewDisabledParams()),
		fx.Provide(func() demultiplexerimpl.Params {
			params := demultiplexerimpl.NewDefaultParams()
			params.UseEventPlatformForwarder = false
			return params
		}),

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
