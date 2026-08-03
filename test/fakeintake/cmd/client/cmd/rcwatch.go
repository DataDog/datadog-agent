// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRCWatchCommand returns the `rc watch` subcommand: a continuous tail
// printing one status line per tick plus diffs of the config set.
func NewRCWatchCommand(cl **client.Client) *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Continuously tail Remote Config state (poll counts + config set diff)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			prev := map[string]struct{}{}
			for {
				stats, err := (*cl).RCStats()
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), err)
				} else {
					cfgs, _ := (*cl).RCListConfigs()
					curr := configKeySet(cfgs)
					added, removed := diffSets(prev, curr)
					sort.Strings(added)
					sort.Strings(removed)
					last := "never"
					if !stats.LastPoll.IsZero() {
						last = fmt.Sprintf("%.1fs ago", time.Since(stats.LastPoll).Seconds())
					}
					fmt.Fprintf(cmd.OutOrStdout(),
						"%s polls=%d last=%s version=%d configs=%d",
						time.Now().Format("15:04:05"),
						stats.Polls, last, stats.Version, stats.ConfigsCount,
					)
					for _, k := range added {
						fmt.Fprintf(cmd.OutOrStdout(), " +%s", k)
					}
					for _, k := range removed {
						fmt.Fprintf(cmd.OutOrStdout(), " -%s", k)
					}
					fmt.Fprintln(cmd.OutOrStdout())
					prev = curr
				}
				<-ticker.C
			}
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 1*time.Second, "tick interval")
	return cmd
}

func configKeySet(cfgs []api.RCConfig) map[string]struct{} {
	out := make(map[string]struct{}, len(cfgs))
	for _, c := range cfgs {
		out[fmt.Sprintf("%s/%s/%s/%s", c.OrgID, c.Product, c.ConfigID, c.ConfigName)] = struct{}{}
	}
	return out
}

func diffSets(prev, curr map[string]struct{}) (added, removed []string) {
	for k := range curr {
		if _, ok := prev[k]; !ok {
			added = append(added, k)
		}
	}
	for k := range prev {
		if _, ok := curr[k]; !ok {
			removed = append(removed, k)
		}
	}
	return
}
