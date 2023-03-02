// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// MakeCommand returns the start subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Gather Datadog trace-agent information.",
		Long:  `Use this to gather informaiton on the runnint trace-agent`,
		RunE: func(*cobra.Command, []string) error {
			return RunTraceAgentInfoFct(globalParamsGetter(), agentInfo)
		},
	}

	return infoCmd
}

func RunTraceAgentInfoFct(params *subcommands.GlobalParams, fct interface{}) error {
	return fxutil.OneShot(fct,
		fx.Supply(config.NewParams(config.WithTraceConfFilePath(params.ConfPath))),
		config.Module,
		fx.Supply(coreconfig.NewAgentParamsWithSecrets(params.ConfPath)),
		coreconfig.Module,
		// fx.Supply(log.LogForOneShot(params.LoggerName, "off", true)),
		// log.Module,
	)
}

// func agentInfo(log log.Component, config config.Component) error {
func agentInfo(config config.Component) error {
	tracecfg := config.Object()
	if tracecfg == nil {
		return fmt.Errorf("Unable to successfully parse config")
	}

	if err := info.InitInfo(tracecfg); err != nil {
		return err
	}

	return info.Info(os.Stdout, tracecfg)
}
