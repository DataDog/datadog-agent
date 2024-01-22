// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package cmd package for the fakeintake client CLI
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewCommand returns the root command for the fakeintakectl CLI
func NewCommand() (cmd *cobra.Command) {
	var url string
	var cl *client.Client

	cmd = &cobra.Command{
		Use:          "fakeintakectl",
		Short:        "fake intake client CLI",
		Long:         `fakeintakectl is a CLI for interacting with fake intake servers.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cl = client.NewClient(url)

			return cl.GetServerHealth()
		},
	}

	cmd.AddCommand(
		NewFilterCommand(&cl),
		NewFlushServerAndResetAggregatorsCommand(&cl),
		NewGetCommand(&cl),
		NewRouteStatsCommand(&cl),
	)

	cmd.PersistentFlags().StringVar(&url, "url", "", "fake intake server url")
	if err := cmd.MarkPersistentFlagRequired("url"); err != nil {
		panic(err)
	}

	return cmd
}
