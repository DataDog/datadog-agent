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

// NewGetCheckRunCommand returns the get check-run command
func NewGetCheckRunCommand(cl **client.Client) (cmd *cobra.Command) {
	var name string

	cmd = &cobra.Command{
		Use:   "check-run",
		Short: "Get check-run",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			checkRun, err := (*cl).GetCheckRun(name)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(checkRun, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the check-run to get")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}

	cmd.AddCommand(
		NewGetCheckRunNamesCommand(cl),
	)

	return cmd
}
