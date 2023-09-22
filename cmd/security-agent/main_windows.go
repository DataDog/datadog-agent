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
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

type service struct {
}

var defaultSecurityAgentConfigFilePaths = []string{
	path.Join(commonpath.DefaultConfPath, "datadog.yaml"),
	path.Join(commonpath.DefaultConfPath, "security-agent.yaml"),
}

// Name returns the service name
func (s *service) Name() string {
	return saconfig.ServiceName
}

// Init does nothing for now.
func (s *service) Init() error {
	return nil
}

// Run actually runs the service; blocks until service exits.
func (s *service) Run(svcctx context.Context) error {

	err := fxutil.OneShot(
		func(log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, telemetry telemetry.Component, forwarder defaultforwarder.Component, pidfilePath string) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer start.StopAgent(cancel, log)

			err := start.RunAgent(ctx, log, config, sysprobeconfig, telemetry, forwarder, "")
			if err != nil {
				return err
			}

			// Wait for stop signal
			<-svcctx.Done()
			log.Info("Received stop from service manager, shutting down...")

			return nil
		},
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewSecurityAgentParams(defaultSecurityAgentConfigFilePaths),
			LogParams:    log.LogForDaemon(command.LoggerName, "security_agent.log_file", pkgconfig.DefaultSecurityAgentLogFile),
		}),
		core.Bundle,
		forwarder.Bundle,
		fx.Provide(defaultforwarder.NewParamsWithResolvers),
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
