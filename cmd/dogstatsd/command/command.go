// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run ../../pkg/config/render_config.go dogstatsd ../../pkg/config/config_template.yaml ./dist/dogstatsd.yaml

package command

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/dogstatsd/subcommands/start"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"

	// register all workloadmeta collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors"
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
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.Agent()
			fmt.Println(fmt.Sprintf("DogStatsD from Agent %s - Codename: %s - Commit: %s - Serialization version: %s - Go version: %s",
				av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion, runtime.Version()))
		},
	}

	return []*cobra.Command{start.Command(defaultLogFile), versionCmd}
}
