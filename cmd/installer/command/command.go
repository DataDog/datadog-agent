// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `installer` binary, including its subcommands.
package command

import (
	"os"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
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

	// NoColor is a flag to disable color output
	NoColor bool
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{
		ConfFilePath: pkgconfigsetup.DefaultUpdaterLogFile,
	}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		Use:   os.Args[0] + " [command]",
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
		&cobra.Group{
			ID:    "extension",
			Title: "Extensions Commands",
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
		if globalParams.NoColor {
			color.NoColor = true
		}
	}

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			agentCmd.AddCommand(cmd)
		}
	}

	if runtime.GOOS == "windows" {
		// Run the default command when no subcommands are provided
		// Intended as shortcut for running the executable as the install script.
		// default: setup --flavor default
		// TODO: Specific to Windows for now, as Linux needs more testing/validation of
		//       the additional migration cases, and the main setup entrypoint is
		//       currently `install.sh` not the `installer` binary.
		agentCmd.Annotations = map[string]string{
			commands.AnnotationHumanReadableErrors: "true",
		}
		agentCmd.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cmd.SetArgs([]string{"setup", "--flavor", "default"})
				return cmd.Execute()
			}
			return nil
		}
	}

	return agentCmd
}
