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

// NewFilterContainerImagesCommand returns the filter container-images command
func NewFilterContainerImagesCommand(cl **client.Client) (cmd *cobra.Command) {
	var name string
	var tags []string

	cmd = &cobra.Command{
		Use:     "container-images",
		Short:   "Filter container images",
		Example: `fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com filter container-images --name gcr.io/datadoghq/agent`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var opts []client.MatchOpt[*aggregator.ContainerImagePayload]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.ContainerImagePayload](tags))
			}

			images, err := (*cl).FilterContainerImages(name, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(images, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name of the container image")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")

	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}

	return cmd
}
