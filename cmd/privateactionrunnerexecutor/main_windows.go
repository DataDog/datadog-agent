// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package main is the entrypoint for the private-action-runner executor process.
package main

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	runsubcmd "github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/spf13/cobra"
)

func main() {
	flavor.SetFlavor(flavor.PrivateActionRunner)
	rootCmd := makeCommand()
	os.Exit(runcmd.Run(rootCmd))
}

func makeCommand() *cobra.Command {
	var globalParams command.GlobalParams
	var socketPath string

	rootCmd := &cobra.Command{
		Use:          "privateactionrunner-executor [command]",
		Short:        "Datadog Private Action Runner executor.",
		SilenceUsage: true,
	}
	rootCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	rootCmd.PersistentFlags().StringArrayVarP(&globalParams.ExtraConfFilePath, "extracfgpath", "E", []string{}, "specify additional configuration files to be loaded sequentially after the main datadog.yaml")

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Private Action Runner executor",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runsubcmd.RunPrivateActionExecutor(context.Background(), globalParams.ConfFilePath, globalParams.ExtraConfFilePath, socketPath)
		},
	}
	runCmd.Flags().StringVar(&socketPath, "socket", "", "local executor IPC socket or named pipe path")
	_ = runCmd.MarkFlagRequired("socket")
	rootCmd.AddCommand(runCmd)
	return rootCmd
}
