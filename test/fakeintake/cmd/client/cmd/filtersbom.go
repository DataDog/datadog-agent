// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewFilterSBOMCommand returns the filter sbom command
func NewFilterSBOMCommand(cl **client.Client) (cmd *cobra.Command) {
	var id string
	var tags []string

	cmd = &cobra.Command{
		Use:     "sbom",
		Short:   "Filter SBOMs",
		Example: `fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com filter sbom --id gcr.io/datadoghq/agent@sha256:c08324052945874a0a5fb1ba5d4d5d1d3b8ff1a87b7b3e46810c8cf476ea4c3d`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var opts []client.MatchOpt[*aggregator.SBOMPayload]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.SBOMPayload](tags))
			}

			sboms, err := (*cl).FilterSBOMs(id, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(sboms, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "ID of the SBOM")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")

	if err := cmd.MarkFlagRequired("id"); err != nil {
		panic(err)
	}

	return cmd
}
