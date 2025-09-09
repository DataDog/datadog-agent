// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetHostTags adds a new command to get host tags
func NewGetHostTags(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "host-tags",
		Short: "Get Host tags",
		RunE: func(*cobra.Command, []string) error {
			hosts, err := (*cl).GetHosts()
			if err != nil {
				return err
			}

			for _, host := range hosts {
				hostTags := (*cl).GetHostTags(host)
				output, err := json.MarshalIndent(hostTags, "", "  ")
				if err != nil {
					return err
				}

				fmt.Println(string(output))
			}

			return nil
		},
	}

	return cmd
}
