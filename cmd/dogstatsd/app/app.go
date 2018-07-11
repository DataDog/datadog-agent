// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// DogstatsdCmd is the root command
	DogstatsdCmd = &cobra.Command{
		Use:   "dogstatsd [command]",
		Short: "Datadog dogstatsd at your service.",
		Long: `
DogStatsD accepts custom application metrics points over UDP, and then
periodically aggregates and forwards them to Datadog, where they can be graphed
on dashboards. DogStatsD implements the StatsD protocol, along with a few
extensions for special Datadog features.`,
		PersistentPreRunE: preRun,
	}

	// confFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	confFilePath string
	flagNoColor  bool
)

// preRun takes care of common setup, including for now:
//   - parsin of the configuration
//   - handling of the no-color flag
func preRun(_ *cobra.Command, _ []string) error {
	if flagNoColor {
		color.NoColor = true
	}

	configFound := false
	// a path to the folder containing the config file was passed
	if len(confFilePath) != 0 {
		// we'll search for a config file named `dogstastd.yaml`
		config.Datadog.SetConfigName("dogstatsd")
		config.Datadog.AddConfigPath(confFilePath)
		confErr := config.Load()
		if confErr != nil {
			log.Error(confErr)
		} else {
			configFound = true
		}
	}

	if !configFound {
		log.Infof("Config will be read from env variables")
	}
	return nil
}

func init() {
	DogstatsdCmd.PersistentFlags().StringVarP(&confFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	DogstatsdCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}
