// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run"
)

// Command returns the main cobra config command.
func Command(globalArgs *app.GlobalArgs) *cobra.Command {
	var pidfilePath string

	startCmd := &cobra.Command{
		Use:        "start",
		Deprecated: "Use \"run\" instead to start the Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.Run(globalArgs, cmd, args)
		},
	}
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return startCmd
}
