// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configcheck implements 'agent configcheck'.
package configcheck

import (
	"bytes"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	var withDebug bool

	configCheckCommand := &cobra.Command{
		Use:     "configcheck",
		Aliases: []string{"checkconfig"},
		Short:   "Print all configurations loaded & resolved of a running agent",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := common.SetupConfig(globalParams.ConfFilePath)
			if err != nil {
				return fmt.Errorf("unable to set up global agent configuration: %v", err)
			}

			err = config.SetupLogger(config.CoreLoggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}
			var b bytes.Buffer
			color.Output = &b
			err = flare.GetConfigCheck(color.Output, withDebug)
			if err != nil {
				return fmt.Errorf("unable to get config: %v", err)
			}

			scrubbed, err := scrubber.ScrubBytes(b.Bytes())
			if err != nil {
				return fmt.Errorf("unable to scrub sensitive data configcheck output: %v", err)
			}

			fmt.Println(string(scrubbed))
			return nil
		},
	}
	configCheckCommand.Flags().BoolVarP(&withDebug, "verbose", "v", false, "print additional debug info")
	return []*cobra.Command{configCheckCommand}
}
