// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetContainerImageCommand returns the get container-image command
func NewGetContainerImageCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use: "container-image",
	}

	cmd.AddCommand(
		NewGetContainerImageNamesCommand(cl),
	)

	return cmd
}
