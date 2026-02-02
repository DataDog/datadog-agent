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
		NewGetAPMStatsCommand(cl),
		NewGetCheckRunCommand(cl),
		NewGetConnectionsCommand(cl),
		NewGetContainerImageCommand(cl),
		NewGetContainerLifecycleEventsCommand(cl),
		NewGetContainersCommand(cl),
		NewGetEventCommand(cl),
		NewGetFlareCommand(cl),
		NewGetLogServiceCommand(cl),
		NewGetMetadataCommand(cl),
		NewGetMetricCommand(cl),
		NewGetProcessDiscoveriesCommand(cl),
		NewGetProcessesCommand(cl),
		NewGetSBOMCommand(cl),
		NewGetTracesCommand(cl),
		NewGetHosts(cl),
	)

	return cmd
}
