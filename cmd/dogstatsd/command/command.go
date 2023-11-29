// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	_ "expvar"
	_ "net/http/pprof"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/dogstatsd/subcommands/start"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

func MakeRootCommand(defaultLogFile string) *cobra.Command {
	// dogstatsdCmd is the root command
	dogstatsdCmd := &cobra.Command{
		Use:   "dogstatsd [command]",
		Short: "Datadog dogstatsd at your service.",
		Long: `
DogStatsD accepts custom application metrics points over UDP, and then
periodically aggregates and forwards them to Datadog, where they can be graphed
on dashboards. DogStatsD implements the StatsD protocol, along with a few
extensions for special Datadog features.`,
	}

	for _, cmd := range makeCommands(defaultLogFile) {
		dogstatsdCmd.AddCommand(cmd)
	}

	return dogstatsdCmd
}

func makeCommands(defaultLogFile string) []*cobra.Command {
	return []*cobra.Command{start.MakeCommand(defaultLogFile), version.MakeCommand("DogStatsD")}
}
