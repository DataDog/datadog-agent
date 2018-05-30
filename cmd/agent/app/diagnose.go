// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(diagnoseCommand)
}

var diagnoseCommand = &cobra.Command{
	Use:   "diagnose",
	Short: "Execute some connectivity diagnosis on your system",
	Long:  ``,
	Run:   doDiagnose,
}

func doDiagnose(cmd *cobra.Command, args []string) {
	// Global config setup
	if confFilePath != "" {
		if err := common.SetupConfig(confFilePath); err != nil {
			fmt.Println("Cannot setup config, exiting:", err)
			panic(err)
		}
	}

	if flagNoColor {
		color.NoColor = true
	}

	err := config.SetupLogger(
		config.Datadog.GetString("log_level"),
		common.DefaultLogFile,
		config.GetSyslogURI(),
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("syslog_tls"),
		config.Datadog.GetString("syslog_pem"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		log.Errorf("Error while setting up logging, exiting: %v", err)
	}

	errors, err := diagnose.RunAll(color.Output)
	if err != nil {
		panic(err)
	}
	os.Exit(errors)
}
