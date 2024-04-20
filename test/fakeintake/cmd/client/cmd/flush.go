// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewFlushServerAndResetAggregatorsCommand returns the flush command
func NewFlushServerAndResetAggregatorsCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "flush",
		Short: "Flush the server and reset aggregators",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return (*cl).FlushServerAndResetAggregators()
		},
	}

	return cmd
}
