// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewGetPayloadPageCommand returns one bounded page of raw request evidence.
func NewGetPayloadPageCommand(cl **client.Client) *cobra.Command {
	var endpoint, cursor string
	var limit int
	cmd := &cobra.Command{
		Use:   "payload-page --endpoint ROUTE [--cursor CURSOR] [--limit N]",
		Short: "Get one raw payload page, preserving compressed bodies and request metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			page, err := (*cl).GetRawPayloadPage(endpoint, cursor, limit)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(page)
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Intake route, such as /api/v2/logs")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Previous page's next_cursor; empty starts at retained history")
	cmd.Flags().IntVar(&limit, "limit", 128, "Maximum records (1..128); individual bodies are never truncated")
	_ = cmd.MarkFlagRequired("endpoint")
	return cmd
}
