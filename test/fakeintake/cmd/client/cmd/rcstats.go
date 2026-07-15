// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRCStatsCommand returns the `rc stats` subcommand.
func NewRCStatsCommand(cl **client.Client) *cobra.Command {
	var watch bool
	var interval time.Duration
	var rootYAML bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show Remote Config poll counters and signing key info",
		RunE: func(cmd *cobra.Command, _ []string) error {
			run := func() error {
				stats, err := (*cl).RCStats()
				if err != nil {
					return err
				}
				if rootYAML {
					fmt.Fprintf(cmd.OutOrStdout(), "remote_configuration:\n  config_root: '%s'\n  director_root: '%s'\n", stats.RootJSON, stats.RootJSON)
					return nil
				}
				return printStats(cmd.OutOrStdout(), stats)
			}
			if !watch {
				return run()
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				fmt.Fprint(cmd.OutOrStdout(), "\033[2J\033[H")
				if err := run(); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), err)
				}
				<-ticker.C
			}
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "refresh continuously")
	cmd.Flags().DurationVar(&interval, "interval", 1*time.Second, "watch interval")
	cmd.Flags().BoolVar(&rootYAML, "root-yaml", false, "print the datadog.yaml snippet (config_root + director_root) for paste-in")
	return cmd
}

func printStats(w interface{ Write([]byte) (int, error) }, s api.RCStats) error {
	out := struct {
		Polls         uint64 `json:"polls"`
		LastPollAgoMS int64  `json:"last_poll_ago_ms,omitempty"`
		LastPoll      string `json:"last_poll,omitempty"`
		Version       uint64 `json:"version"`
		ConfigsCount  int    `json:"configs_count"`
		KeyID         string `json:"key_id"`
		PublicKey     string `json:"public_key"`
	}{
		Polls:        s.Polls,
		Version:      s.Version,
		ConfigsCount: s.ConfigsCount,
		KeyID:        s.KeyID,
		PublicKey:    s.PublicKey,
	}
	if !s.LastPoll.IsZero() {
		out.LastPoll = s.LastPoll.Format(time.RFC3339)
		out.LastPollAgoMS = time.Since(s.LastPoll).Milliseconds()
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
