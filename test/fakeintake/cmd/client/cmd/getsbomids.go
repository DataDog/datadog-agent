// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetSBOMIDsCommand returns the get container-image names command
func NewGetSBOMIDsCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "ids",
		Short: "Get SBOM IDs",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := (*cl).GetSBOMIDs()
			if err != nil {
				return err
			}

			for _, id := range ids {
				fmt.Println(id)
			}

			return nil
		},
	}

	return cmd
}
