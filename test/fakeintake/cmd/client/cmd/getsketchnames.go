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

// NewGetSketchNamesCommand returns the get sketch names command
func NewGetSketchNamesCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "names",
		Short: "Get sketch names",
		RunE: func(*cobra.Command, []string) error {
			names, err := (*cl).GetSketchNames()
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
