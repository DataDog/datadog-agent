// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	diagnoseCommand = &cobra.Command{
		Use:   "diagnose",
		Short: "Check availability of cloud provider metadata endpoints",
		Long:  ``,
		RunE:  doDiagnoseMetadataAvailability,
	}

	diagnoseMetadataAvailabilityCommand = &cobra.Command{
		Use:   "metadata-availability",
		Short: "Check availability of cloud provider metadata endpoints",
		Long:  ``,
		RunE:  doDiagnoseMetadataAvailability,
	}

	diagnoseDatadogConnectivityCommand = &cobra.Command{
		Use:    "datadog-connectivity",
		Short:  "Check connectivity between your system and Datadog endpoints",
		Long:   ``,
		Hidden: true,
		RunE:   doDiagnoseDatadogConnectivity,
	}
)

func init() {

	diagnoseCommand.AddCommand(diagnoseMetadataAvailabilityCommand)
	diagnoseCommand.AddCommand(diagnoseDatadogConnectivityCommand)

	AgentCmd.AddCommand(diagnoseCommand)
}

func doDiagnoseMetadataAvailability(cmd *cobra.Command, args []string) error {
	// Global config setup
	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	if flagNoColor {
		color.NoColor = true
	}

	err = config.SetupLogger(
		loggerName,
		config.Datadog.GetString("log_level"),
		common.DefaultLogFile,
		config.GetSyslogURI(),
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		return fmt.Errorf("error while setting up logging, exiting: %v", err)
	}

	return diagnose.RunAll(color.Output)
}

func doDiagnoseDatadogConnectivity(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("this command is not implemented yet")
}
