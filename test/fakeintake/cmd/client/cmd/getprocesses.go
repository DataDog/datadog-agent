// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetProcessesCommand returns the get processes command
func NewGetProcessesCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "processes",
		Short: "Get processes",
		RunE: func(*cobra.Command, []string) error {
			procs, err := (*cl).GetProcesses()
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(procs, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	return cmd
}
