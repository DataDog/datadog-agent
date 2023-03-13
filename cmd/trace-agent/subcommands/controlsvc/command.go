// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

// Package controlsvc implements 'agent start-service', 'agent stopservice',
// and 'agent restart-service'.
package controlsvc

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/windows/controlsvc"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type RunParams struct {
	*subcommands.GlobalParams

	// Interactive sets whether or not to run the agent in interactive mode.
	Interactive bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParamsGetter func() *subcommands.GlobalParams) []*cobra.Command {
	cliParams := &RunParams{}
	startCmd := &cobra.Command{
		Use:     "start-service",
		Aliases: []string{"startservice"},
		Short:   "starts the agent within the service control manager",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.GlobalParams = globalParamsGetter()
			return fxutil.OneShot(controlsvc.StartService)
		},
	}
	startCmd.PersistentFlags().BoolVarP(&cliParams.Interactive, "foreground", "f", false,
		"Always run foreground instead whether session is interactive or not")

	stopCmd := &cobra.Command{

		Use:     "stop-service",
		Aliases: []string{"stopservice"},
		Short:   "stops the agent within the service control manager",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.GlobalParams = globalParamsGetter()
			return fxutil.OneShot(controlsvc.StopService)
		},
	}

	return []*cobra.Command{startCmd, stopCmd}
}
