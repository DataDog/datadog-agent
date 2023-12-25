// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command holds the main command factory for CWS profiler
package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func() []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	// cwsProfilerCmd is the root command
	cwsProfilerCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent CWS workload profiler",
		Long: `
The Datadog Agent CWS workload profiler is used to learn a workload by listening from cws-instrumenatation ptracer; then it saves it as security profile.`,
		SilenceUsage: true,
	}

	for _, sf := range subcommandFactories {
		subcommands := sf()
		for _, cmd := range subcommands {
			cwsProfilerCmd.AddCommand(cmd)
		}
	}

	return cwsProfilerCmd
}
