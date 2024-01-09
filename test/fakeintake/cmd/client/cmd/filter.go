// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewFilterCommand returns the filter command
func NewFilterCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "filter",
		Short: "Filter metrics, logs, etc.",
	}

	cmd.AddCommand(
		NewFilterLogsCommand(cl),
		NewFilterMetricsCommand(cl),
		NewFilterContainerImagesCommand(cl),
		NewFilterSBOMCommand(cl),
	)

	return cmd
}
