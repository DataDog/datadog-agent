// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package command holds command related files for par-executor.
package command

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// LoggerName is the logger name for par-executor.
const LoggerName = "PAR-EXECUTOR"

// GlobalParams contains the values of global Cobra flags.
type GlobalParams struct {
	ConfFilePath       string
	ExtraConfFilePath  []string
	SocketPath         string
	IdleTimeoutSeconds int
	NoColor            bool
}

// SubcommandFactory returns subcommands bound to GlobalParams.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand builds the root cobra command for par-executor.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	var globalParams GlobalParams

	rootCmd := &cobra.Command{
		Use:          "par-executor [command]",
		Short:        "PAR execution-plane binary (spawned by par-control)",
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	rootCmd.PersistentFlags().StringArrayVarP(&globalParams.ExtraConfFilePath, "extracfgpath", "E", []string{}, "additional configuration files")
	rootCmd.PersistentFlags().StringVar(&globalParams.SocketPath, "socket", "", "Unix socket path for IPC with par-control (required)")
	rootCmd.PersistentFlags().IntVar(&globalParams.IdleTimeoutSeconds, "idle-timeout-seconds", 120, "seconds of inactivity before self-terminating (0 = disabled)")
	rootCmd.PersistentFlags().BoolVarP(&globalParams.NoColor, "no-color", "n", false, "disable color output")

	rootCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if globalParams.NoColor {
			color.NoColor = true
		}
	}

	for _, factory := range subcommandFactories {
		for _, subcmd := range factory(&globalParams) {
			rootCmd.AddCommand(subcmd)
		}
	}

	return rootCmd
}
