// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && otlp

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/spf13/cobra"
)

// MakeCommand creates the `run` command
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Starting OpenTelemetry Collector",
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalConfGetter()
			params.GlobalParams = globalParams
			return runOTelAgentCommand(context.Background(), params)
		},
	}
	cmd.Flags().StringVarP(&params.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return cmd
}
