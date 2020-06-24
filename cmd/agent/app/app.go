// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface. */
package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
		SilenceUsage: true,
	}
	// confFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	confFilePath string
	flagNoColor  bool
	// sysProbeConfFilePath holds the path to the folder containing the system-probe
	// configuration file, to allow overrides from the command line
	sysProbeConfFilePath string
)

// loggerName is the name of the core agent logger
const loggerName config.LoggerName = "CORE"

func init() {
	AgentCmd.PersistentFlags().StringVarP(&confFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	AgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
	AgentCmd.PersistentFlags().StringVarP(&sysProbeConfFilePath, "sysprobecfgpath", "", "", "path to directory containing system-probe.yaml")
}
