// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetEventSourcesCommand returns the get event sources command
func NewGetEventSourcesCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "sources",
		Short: "Get event sources",
		RunE: func(*cobra.Command, []string) error {
			sources, err := (*cl).GetEventSources()
			if err != nil {
				return err
			}

			for _, source := range sources {
				fmt.Println(source)
			}

			return nil
		},
	}

	return cmd
}
