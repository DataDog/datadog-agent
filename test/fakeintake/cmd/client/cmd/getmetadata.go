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

// NewGetMetadataCommand returns the get metadata command
func NewGetMetadataCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "metadata",
		Short: "Get metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			metadata, err := (*cl).GetMetadata()
			if err != nil {
				return err
			}
			for _, payload := range metadata {
				output, err := json.MarshalIndent(payload, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(output))
			}
			return nil
		},
	}

	return cmd
}
