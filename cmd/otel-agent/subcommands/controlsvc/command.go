// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && otlp

// Package controlsvc implements 'otel-agent start-service', 'otel-agent stop-service',
// and 'otel-agent restart-service'.
package controlsvc

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/windows/controlsvc"
)

// Commands returns a slice of subcommands for the 'otel-agent' command.
func Commands(_ *subcommands.GlobalParams) []*cobra.Command {
	return []*cobra.Command{
		{
			Use:     "start-service",
			Aliases: []string{"startservice"},
			Short:   "starts the otel-agent within the service control manager",
			Long:    ``,
			RunE: func(_ *cobra.Command, _ []string) error {
				return controlsvc.StartService()
			},
		},
		{
			Use:     "stop-service",
			Aliases: []string{"stopservice"},
			Short:   "stops the otel-agent within the service control manager",
			Long:    ``,
			RunE: func(_ *cobra.Command, _ []string) error {
				return controlsvc.StopService()
			},
		},
		{
			Use:     "restart-service",
			Aliases: []string{"restartservice"},
			Short:   "restarts the otel-agent within the service control manager",
			Long:    ``,
			RunE: func(_ *cobra.Command, _ []string) error {
				return controlsvc.RestartService()
			},
		},
	}
}
