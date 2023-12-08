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

// NewGetFlareCommand returns the get flare command
func NewGetFlareCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "flare",
		Short: "Get flare",
		RunE: func(cmd *cobra.Command, args []string) error {
			flare, err := (*cl).GetLatestFlare()
			if err != nil {
				return err
			}

			fmt.Println(flare.GetFilenames())

			return nil
		},
	}

	return cmd
}
