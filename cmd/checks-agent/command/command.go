// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package command

import (
	//nolint:revive // TODO(AML) Fix revive linter
	_ "expvar"
	_ "net/http/pprof"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/checks-agent/subcommands/start"
	"github.com/DataDog/datadog-agent/cmd/checks-agent/subcommands/version"
)

func MakeRootCommand() *cobra.Command {
	// checksAgentCmd is the root command
	checksAgentCmd := &cobra.Command{
		Use:   "checks-agent [command]",
		Short: "Checks Agent at your service.",
	}

	for _, cmd := range makeCommands() {
		checksAgentCmd.AddCommand(cmd)
	}

	return checksAgentCmd
}

func makeCommands() []*cobra.Command {
	return []*cobra.Command{start.MakeCommand(), version.MakeCommand("Checks Agent")}
}
