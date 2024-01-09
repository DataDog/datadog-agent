// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRouteStatsCommand returns the route-stats command
func NewRouteStatsCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "route-stats",
		Short: "Get route stats",
		Run: func(cmd *cobra.Command, args []string) {
			stats, err := (*cl).RouteStats()
			if err != nil {
				log.Fatalln(err)
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Route", "Count"})
			for route, count := range stats {
				table.Append([]string{route, fmt.Sprintf("%d", count)})
			}
			table.Render()
		},
	}

	return cmd
}
