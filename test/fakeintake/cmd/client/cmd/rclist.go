// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRCListCommand returns the `rc list` subcommand.
func NewRCListCommand(cl **client.Client) *cobra.Command {
	var pretty bool
	var configID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Remote Config entries",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfgs, err := (*cl).RCListConfigs()
			if err != nil {
				return err
			}
			if configID != "" {
				filtered := cfgs[:0]
				for _, c := range cfgs {
					if c.ConfigID == configID {
						filtered = append(filtered, c)
					}
				}
				cfgs = filtered
			}
			if !pretty {
				type rcConfigOut struct {
					Key        string          `json:"key"`
					OrgID      string          `json:"org_id"`
					Product    string          `json:"product"`
					ConfigID   string          `json:"config_id"`
					ConfigName string          `json:"config_name"`
					Data       json.RawMessage `json:"data"`
				}
				rendered := make([]rcConfigOut, len(cfgs))
				for i, c := range cfgs {
					rendered[i] = rcConfigOut{
						Key:        fmt.Sprintf("%s/%s/%s/%s", c.OrgID, c.Product, c.ConfigID, c.ConfigName),
						OrgID:      c.OrgID,
						Product:    c.Product,
						ConfigID:   c.ConfigID,
						ConfigName: c.ConfigName,
						Data:       json.RawMessage(c.Data),
					}
				}
				out, err := json.MarshalIndent(rendered, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			return printPretty(cmd.OutOrStdout(), cfgs)
		},
	}
	cmd.Flags().BoolVar(&pretty, "pretty", false, "pretty-print stored JSON config bodies")
	cmd.Flags().StringVar(&configID, "config-id", "", "filter to a single config_id")
	return cmd
}

func printPretty(w interface{ Write([]byte) (int, error) }, cfgs []api.RCConfig) error {
	for _, c := range cfgs {
		var v interface{}
		body := c.Data
		if err := json.Unmarshal(body, &v); err == nil {
			b, _ := json.MarshalIndent(v, "  ", "  ")
			body = b
		}
		fmt.Fprintf(w, "[%s] %s/%s/%s/%s\n  %s\n", c.Product, c.OrgID, c.Product, c.ConfigID, c.ConfigName, body)
	}
	return nil
}
