// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewFilterHistogramsCommand returns the filter histograms command
func NewFilterHistogramsCommand(cl **client.Client) (cmd *cobra.Command) {
	var name string
	var tags []string

	cmd = &cobra.Command{
		Use:     "histograms",
		Short:   "Filter OTel histograms (explicit bucket and exponential)",
		Example: "fakeintakectl --url http://localhost:8080 filter histograms --name http.server.request.duration --tags env:prod",
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			var opts []client.MatchOpt[*aggregator.Histogram]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.Histogram](tags))
			}

			histograms, err := (*cl).FilterHistograms(name, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(histograms, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the histogram metric")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")

	if err := cmd.MarkFlagRequired("name"); err != nil {
		return nil
	}

	return cmd
}
