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

// NewGetProcessDiscoveriesCommand returns the get process-discoveries command
func NewGetProcessDiscoveriesCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "process-discoveries",
		Short: "Get process discoveries",
		RunE: func(cmd *cobra.Command, args []string) error {
			pd, err := (*cl).GetProcessDiscoveries()
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(pd, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	return cmd
}
