// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface. */
package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   "integrations [command]",
		Short: "Datadog Agent integrations.",
		Long: `
Manage your Datadog Agent integrations.`,
	}
	// confFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	confFilePath string
	flagNoColor  bool
)

func init() {
	fmt.Printf("hello app init\n")
	AgentCmd.PersistentFlags().StringVarP(&confFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	AgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}
