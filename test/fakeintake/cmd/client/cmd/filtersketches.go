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

// NewFilterSketchesCommand returns the filter sketches command
func NewFilterSketchesCommand(cl **client.Client) (cmd *cobra.Command) {
	var name string
	var tags []string

	cmd = &cobra.Command{
		Use:     "sketches",
		Short:   "Filter sketches (distribution metrics)",
		Example: "fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com filter sketches --name my.distribution --tags env:prod",
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			var opts []client.MatchOpt[*aggregator.Sketch]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.Sketch](tags))
			}

			sketches, err := (*cl).FilterSketches(name, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(sketches, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the sketch metric")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")

	if err := cmd.MarkFlagRequired("name"); err != nil {
		return nil
	}

	return cmd
}
