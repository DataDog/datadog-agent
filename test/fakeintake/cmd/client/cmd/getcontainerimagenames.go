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

// NewGetContainerImageNamesCommand returns the get container-image names command
func NewGetContainerImageNamesCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "names",
		Short: "Get container image names",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := (*cl).GetContainerImageNames()
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
