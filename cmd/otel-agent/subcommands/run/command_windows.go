// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && otlp

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
	"github.com/spf13/cobra"
)

// MakeCommand creates the 'run' command on Windows
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Starting OpenTelemetry Collector",
		RunE: func(_ *cobra.Command, _ []string) error {
			params.GlobalParams = globalConfGetter()
			if servicemain.RunningAsWindowsService() {
				servicemain.Run(&service{cliParams: params})
				return nil
			}
			return runOTelAgentCommand(context.Background(), params)
		},
	}
	return cmd
}
