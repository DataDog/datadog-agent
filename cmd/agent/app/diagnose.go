// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

var verbose bool

func init() {
	AgentCmd.AddCommand(diagnoseCommand)
	flag.BoolVarP(&verbose, "verbose", "v", false, "verbose output (with logs)")
}

var diagnoseCommand = &cobra.Command{
	Use:   "diagnose",
	Short: "Execute some connectivity diagnosis on your system",
	Long:  ``,
	RunE:  doDiagnose,
}

func doDiagnose(cmd *cobra.Command, args []string) error {
	if flagNoColor {
		color.NoColor = true
	}

	if verbose {
		config.SetupLogger("debug", "", "", false, false, "", true)
	} else {
		config.SetupLogger("off", "", "", false, false, "", false)
	}

	diagnose.Diagnose(color.Output)
	return nil
}
