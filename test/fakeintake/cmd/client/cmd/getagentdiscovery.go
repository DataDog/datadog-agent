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

// NewGetAgentDiscoveryCommand returns the get agent-discovery command.
func NewGetAgentDiscoveryCommand(cl **client.Client) (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:   "agent-discovery",
		Short: "Get Agent Discovery payloads",
		RunE: func(*cobra.Command, []string) error {
			payloads, err := (*cl).GetAgentDiscoveryPayloads()
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(payloads, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	return cmd
}
