// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command holds the main command factory for CWS injector
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
	// cwsInjectorCmd is the root command
	cwsInjectorCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent CWS Injector",
		Long: `
The Datadog Agent CWS Injector is used for multiple purposes:
1/ to instrument remote User Sessions (like SSH sessions or Kubernetes
   remote access sessions) so that CWS can enrich its events with the
   real user context.
2/ to trace workloads using ptrace when EBPF is not available.`,
		SilenceUsage: true,
	}

	for _, sf := range subcommandFactories {
		subcommands := sf()
		for _, cmd := range subcommands {
			cwsInjectorCmd.AddCommand(cmd)
		}
	}

	return cwsInjectorCmd
}
