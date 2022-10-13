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

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/controlsvc"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	return []*cobra.Command{
		{
			Use:   "start-service",
			Short: "starts the agent within the service control manager",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(controlsvc.StartService)
			},
		},
		{
			Use:   "stopservice",
			Short: "stops the agent within the service control manager",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(controlsvc.StopService)
			},
		},
		{
			Use:   "restart-service",
			Short: "restarts the agent within the service control manager",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(controlsvc.RestartService)
			},
		},
	}
}
