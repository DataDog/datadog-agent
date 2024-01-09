// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetCommand returns the get command
func NewGetCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use: "get",
	}

	cmd.AddCommand(
		NewGetCheckRunCommand(cl),
		NewGetConnectionsCommand(cl),
		NewGetContainerLifecycleEventsCommand(cl),
		NewGetContainerImageCommand(cl),
		NewGetContainersCommand(cl),
		NewGetFlareCommand(cl),
		NewGetLogServiceCommand(cl),
		NewGetMetricCommand(cl),
		NewGetProcessDiscoveriesCommand(cl),
		NewGetProcessesCommand(cl),
		NewGetSBOMCommand(cl),
	)

	return cmd
}
