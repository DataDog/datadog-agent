// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface.
*/
package app

import (
	"github.com/spf13/cobra"
)

var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
	}
	// confFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	confFilePath string
)

func init() {
	startCmd.Flags().StringVarP(&confFilePath, "cfgpath", "f", "", "path to directory containing datadog.yaml")
}
