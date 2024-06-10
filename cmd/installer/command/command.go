// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `installer` binary, including its subcommands.
package command

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// common constants for all the updater subcommands.
const (
	ConfigName      = "installer"
	LoggerName      = "INSTALLER"
	DefaultLogLevel = "off"
)

// GlobalParams contains the values of installer-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

	// LogFilePath is the path to the log file.
	LogFilePath string

	// PIDFilePath is the path to the pidfile.
	PIDFilePath string

	// AllowNoRoot is a flag to allow running the installer as non-root.
	AllowNoRoot bool
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{
		ConfFilePath: config.DefaultUpdaterLogFile,
	}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Installer at your service.",
		Long: `
Datadog Installer installs datadog-packages based on your commands.`,
		SilenceUsage: true,
	}
	agentCmd.AddGroup(
		&cobra.Group{
			ID:    "installer",
			Title: "Installer Commands",
		},
		&cobra.Group{
			ID:    "daemon",
			Title: "Daemon Commands",
		},
		&cobra.Group{
			ID:    "bootstrap",
			Title: "Bootstrap Commands",
		},
		&cobra.Group{
			ID:    "apm",
			Title: "APM Commands",
		},
	)

	agentCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing installer.yaml")
	agentCmd.PersistentFlags().StringVarP(&globalParams.PIDFilePath, "pidfile", "p", "", "path to the pidfile")
	agentCmd.PersistentFlags().StringVarP(&globalParams.LogFilePath, "logfile", "l", "", "path to the logfile")
	agentCmd.PersistentFlags().BoolVar(&globalParams.AllowNoRoot, "no-root", false, "allow running the installer as non-root")

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
