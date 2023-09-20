// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/trace"
	"github.com/DataDog/datadog-agent/comp/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MakeCommand returns the run subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {

	cliParams := &RunParams{}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start datadog trace-agent.",
		Long:  `The Datadog trace-agent aggregates, samples, and forwards traces to datadog submitted by tracers loaded into your application.`,
		RunE: func(*cobra.Command, []string) error {
			cliParams.GlobalParams = globalParamsGetter()
			return runTraceAgent(cliParams, cliParams.ConfPath)
		},
	}

	setParamFlags(runCmd, cliParams)

	return runCmd
}

func setParamFlags(cmd *cobra.Command, cliParams *RunParams) {
	cmd.PersistentFlags().StringVarP(&cliParams.PIDFilePath, "pidfile", "p", "", "path for the PID file to be created")
	cmd.PersistentFlags().StringVarP(&cliParams.CPUProfile, "cpu-profile", "l", "",
		"enables CPU profiling and specifies profile path.")
	cmd.PersistentFlags().StringVarP(&cliParams.MemProfile, "mem-profile", "m", "",
		"enables memory profiling and specifies profile.")

	setOSSpecificParamFlags(cmd, cliParams)
}

func runFx(cliParams *RunParams, defaultConfPath string) error {
	if cliParams.ConfPath == "" {
		cliParams.ConfPath = defaultConfPath
	}
	return fxutil.Run(
		fx.Supply(coreconfig.NewAgentParamsWithSecrets(cliParams.ConfPath)),
		coreconfig.Module,
		// Required to avoid cyclic imports.
		fx.Supply(agent.Params{
			CPUProfile:  cliParams.CPUProfile,
			MemProfile:  cliParams.MemProfile,
			PIDFilePath: cliParams.PIDFilePath,
		}),
		trace.Bundle,
	)
}
