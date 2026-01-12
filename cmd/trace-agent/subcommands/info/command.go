// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package info contains the 'info' subcommand for the 'trace-agent' command.
package info

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-noop"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MakeCommand returns the start subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Gather Datadog trace-agent information.",
		Long:  `Use this to gather information on the running trace-agent`,
		RunE: func(*cobra.Command, []string) error {
			return runTraceAgentInfoFct(globalParamsGetter(), agentInfo)
		},
	}

	return infoCmd
}

func runTraceAgentInfoFct(params *subcommands.GlobalParams, fct interface{}) error {
	return fxutil.OneShot(fct,
		config.Module(),
		fx.Supply(coreconfig.NewAgentParams(params.ConfPath, coreconfig.WithFleetPoliciesDirPath(params.FleetPoliciesDirPath))),
		fx.Supply(log.ForOneShot(params.LoggerName, "off", true)),
		secretsnoopfx.Module(),
		coreconfig.Module(),
		nooptagger.Module(),
		ipcfx.ModuleReadOnly(),
		logfx.Module(),
	)
}

func agentInfo(config config.Component) error {
	tracecfg := config.Object()
	if tracecfg == nil {
		return errors.New("Unable to successfully parse config")
	}
	if err := info.InitInfo(tracecfg); err != nil {
		return err
	}
	return info.Info(os.Stdout, tracecfg)
}
