// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

// Package controlsvc implements 'trace-agent start-service', 'trace-agent stop-service',
// and 'trace-agent restart-service'.
package controlsvc

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/windows/controlsvc"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns a slice of subcommands for the 'trace-agent' command.
func Commands(globalParamsGetter func() *subcommands.GlobalParams) []*cobra.Command { //nolint:revive // TODO fix revive unused-parameter
	startCmd := &cobra.Command{
		Use:     "start-service",
		Aliases: []string{"startservice"},
		Short:   "starts the agent within the service control manager",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(controlsvc.StartService)
		},
	}

	stopCmd := &cobra.Command{

		Use:     "stop-service",
		Aliases: []string{"stopservice"},
		Short:   "stops the agent within the service control manager",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(controlsvc.StopService)
		},
	}

	restartCmd := &cobra.Command{

		Use:     "restart-service",
		Aliases: []string{"restartservice"},
		Short:   "restarts the agent within the service control manager",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(controlsvc.RestartService)
		},
	}

	return []*cobra.Command{startCmd, stopCmd, restartCmd}
}
