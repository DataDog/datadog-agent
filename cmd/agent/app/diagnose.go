// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

var quiet bool

func init() {
	AgentCmd.AddCommand(diagnoseCommand)
	flag.BoolVarP(&quiet, "quiet", "q", false, "remove logs from output")
}

var diagnoseCommand = &cobra.Command{
	Use:   "diagnose",
	Short: "Execute some connectivity diagnosis on your system",
	Long:  ``,
	Run:   doDiagnose,
}

func doDiagnose(cmd *cobra.Command, args []string) {
	if flagNoColor {
		color.NoColor = true
	}

	if quiet {
		config.SetupLogger("off", "", "", false, false, "", false)
	} else {
		config.SetupLogger("debug", "", "", false, false, "", true)
	}

	errors := diagnose.Diagnose(color.Output)
	os.Exit(errors)
}
