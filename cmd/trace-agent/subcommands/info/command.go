// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
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
		// fx.Supply(log.LogForOneShot(params.LoggerName, "off", true)),
		// log.Module,
	)
}

// func agentInfo(log log.Component, config config.Component) error {
func agentInfo(config config.Component) error {
	// cfg, err := config.LoadConfigFile()
	// if err != nil {
	// 	fmt.Println(err) // TODO: remove me
	// 	if err == tracecfg.ErrMissingAPIKey {
	// 		fmt.Println(tracecfg.ErrMissingAPIKey)

	// 		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
	// 		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
	// 		// http://supervisord.org/subprocess.html#process-states
	// 		time.Sleep(5 * time.Second)

	// 		// Don't use os.Exit() method here, even with os.Exit(0) the Service Control Manager
	// 		// on Windows will consider the process failed and log an error in the Event Viewer and
	// 		// attempt to restart the process.
	// 		return err
	// 	}
	// 	return err
	// }

	info.InitInfo(config.Object())
	return info.Info(os.Stdout, config.Object())
}
