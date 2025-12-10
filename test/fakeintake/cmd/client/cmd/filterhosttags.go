// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

func NewFilterHostTagsCommand(cl **client.Client) (cmd *cobra.Command) {
	var hostFlagName = "host"
	var host string

	cmd = &cobra.Command{
		Use:     "host-tags",
		Short:   "Filter host-tags",
		Example: "fakeintakectl --url http://internal-crayon-gcp-fakeintake.gcp.cloud filter host-tags --host my-gcp-host",
		RunE: func(cmd *cobra.Command, _ []string) error {
			hostTags, err := (*cl).GetHostTags(host)
			if err != nil {
				return fmt.Errorf("failed to get host-tags for host %s: %w", host, err)
			}

			for _, hostTag := range hostTags {
				hostTagStr, err := json.MarshalIndent(hostTag, "", "  ")
				if err != nil {
					cmd.PrintErrf("failed to format hostTag '%v' : %s", hostTag, err.Error())
					continue
				}

				cmd.Println(string(hostTagStr))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&host, hostFlagName, "", "hostname to get host-tags from")

	if err := cmd.MarkFlagRequired(hostFlagName); err != nil {
		// only way this can fail is if the flag does not exist.
		panic(err)
	}

	return cmd
}
