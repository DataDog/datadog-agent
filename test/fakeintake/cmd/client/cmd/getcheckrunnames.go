// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetCheckRunNamesCommand returns the get check-run names command
func NewGetCheckRunNamesCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "names",
		Short: "Get check-run names",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			names, err := (*cl).GetCheckRunNames()
			if err != nil {
				return err
			}

			for _, name := range names {
				fmt.Println(name)
			}

			return nil
		},
	}

	return cmd
}
