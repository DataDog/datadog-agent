// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetLogServiceCommand returns the get log-service command
func NewGetLogServiceCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use: "log-service",
	}

	cmd.AddCommand(
		NewGetLogServiceNamesCommand(cl),
	)

	return cmd
}
