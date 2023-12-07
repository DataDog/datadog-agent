// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetMetricCommand returns the get metric command
func NewGetMetricCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use: "metric",
	}

	cmd.AddCommand(
		NewGetMetricNamesCommand(cl),
	)

	return cmd
}
