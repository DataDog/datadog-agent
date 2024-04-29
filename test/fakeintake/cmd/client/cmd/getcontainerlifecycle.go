// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetContainerLifecycleEventsCommand returns the filter sbom command
func NewGetContainerLifecycleEventsCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:     "container-lifecycle-events",
		Short:   "Get container lifecycle events",
		Example: `fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com get container-lifecycle-events`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			evts, err := (*cl).GetContainerLifecycleEvents()
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(evts, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	return cmd
}
