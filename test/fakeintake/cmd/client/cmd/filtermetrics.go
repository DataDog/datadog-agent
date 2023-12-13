// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewFilterMetricsCommand returns the filter metrics command
func NewFilterMetricsCommand(cl **client.Client) (cmd *cobra.Command) {
	var name string
	var tags []string
	var min float64
	var max float64

	cmd = &cobra.Command{
		Use:     "metrics",
		Short:   "Filter metrics",
		Example: "fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com filter metrics --name container.cpu.usage --tags image_name:gcr.io/datadoghq/agent,container_name:agent",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var opts []client.MatchOpt[*aggregator.MetricSeries]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.MetricSeries](tags))
			}
			if cmd.Flag("min").Changed {
				opts = append(opts, client.WithMetricValueHigherThan(min))
			}
			if cmd.Flag("max").Changed {
				opts = append(opts, client.WithMetricValueLowerThan(max))
			}

			metrics, err := (*cl).FilterMetrics(name, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(metrics, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the metric")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")
	cmd.Flags().Float64Var(&min, "min", 0, "Minimum value")
	cmd.Flags().Float64Var(&max, "max", 0, "Maximum value")

	if err := cmd.MarkFlagRequired("name"); err != nil {
		return nil
	}

	return cmd
}
