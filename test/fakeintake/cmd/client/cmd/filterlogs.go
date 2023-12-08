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

// NewFilterLogsCommand returns the filter logs command
func NewFilterLogsCommand(cl **client.Client) (cmd *cobra.Command) {
	var service string
	var tags []string
	var content string
	var pattern string

	cmd = &cobra.Command{
		Use:     "logs",
		Short:   "Filter logs",
		Example: `fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com filter logs --service agent --tags image_name:gcr.io/datadoghq/agent,container_name:agent`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var opts []client.MatchOpt[*aggregator.Log]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.Log](tags))
			}
			if cmd.Flag("contains").Changed {
				opts = append(opts, client.WithMessageContaining(content))
			}
			if cmd.Flag("match").Changed {
				opts = append(opts, client.WithMessageMatching(pattern))
			}

			logs, err := (*cl).FilterLogs(service, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(logs, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&service, "service", "", "Name of the service")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")
	cmd.Flags().StringVar(&content, "contains", "", "Content to filter on")
	cmd.Flags().StringVar(&pattern, "match", "", "Pattern to filter on")

	if err := cmd.MarkFlagRequired("service"); err != nil {
		panic(err)
	}

	return cmd
}
