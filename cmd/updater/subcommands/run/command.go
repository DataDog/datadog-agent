// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'updater run'.
package run

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/updater/command"

	"github.com/spf13/cobra"
)

// Commands returns the global params commands
func Commands(_ *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the updater",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			for range time.NewTicker(5 * time.Second).C {
				fmt.Println("updater running")
			}
			return nil
		},
	}
	return []*cobra.Command{runCmd}
}
