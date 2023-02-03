// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks
// +build !windows,clusterchecks

// Package command implements the top-level `cluster-agent-cloudfoundry` binary, including its subcommands.
package command

import (
	"fmt"
	clustercheckscmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/dcaconfigcheck"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/dcaflare"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
)

const (
	LoggerName = "CLUSTER"
)

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Cluster Agent for Cloud Foundry at your service.",
		Long: `
Datadog Cluster Agent for Cloud Foundry takes care of running checks that need to run only
once per cluster.`,
		SilenceUsage: true,
	}

	agentCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog-agent.yaml")

	agentCmd.AddCommand(clustercheckscmd.MakeCommand(func() clustercheckscmd.GlobalParams {
		return clustercheckscmd.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
		}
	}))

	agentCmd.AddCommand(dcaconfigcheck.MakeCommand(func() dcaconfigcheck.GlobalParams {
		return dcaconfigcheck.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
		}
	}))

	agentCmd.AddCommand(dcaflare.MakeCommand(func() dcaflare.GlobalParams {
		return dcaflare.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
		}
	}))
	// github.com/fatih/color sets its global color.NoColor to a default value based on
	// whether the process is running in a tty.  So, we only want to override that when
	// the value is true.
	var noColorFlag bool
	agentCmd.PersistentFlags().BoolVarP(&noColorFlag, "no-color", "n", false, "disable color output")
	agentCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if noColorFlag {
			color.NoColor = true
		}
	}

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			agentCmd.AddCommand(cmd)
		}
	}

	return agentCmd
}
