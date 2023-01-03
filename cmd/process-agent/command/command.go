// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `process-agent` binary, including its subcommands.
package command

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	"github.com/DataDog/datadog-agent/pkg/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
)

const LoggerName config.LoggerName = "PROCESS"

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

	// SysProbeConfFilePath holds the path to the folder containing the system-probe
	// configuration file, to allow overrides from the command line
	SysProbeConfFilePath string

	// PidFilePath specifies the path to the pid file
	PidFilePath string

	// Info
	Info bool

	// WinParams provides windows specific options
	WinParams WinParams
}

// WinParams specifies Windows-specific CLI params
type WinParams struct {
	// StartService handles starting the service
	StartService bool

	// StopService handles stopping the service
	StopService bool

	// Foreground handles running the service in the foreground
	Foreground bool
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory, winParams bool, rootCmdRun func(globalParams *GlobalParams)) *cobra.Command {
	globalParams := GlobalParams{}

	rootCmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			rootCmdRun(&globalParams)
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&globalParams.ConfFilePath, flags.CfgPath, flags.DefaultConfPath, "Path to datadog.yaml config")

	if flags.DefaultSysProbeConfPath != "" {
		rootCmd.PersistentFlags().StringVar(&globalParams.SysProbeConfFilePath, flags.SysProbeConfig, flags.DefaultSysProbeConfPath, "Path to system-probe.yaml config")
	}

	rootCmd.PersistentFlags().StringVarP(&globalParams.PidFilePath, "pid", "p", "", "Path to set pidfile for process")
	rootCmd.PersistentFlags().BoolVarP(&globalParams.Info, "info", "i", false, "Show info about running process agent and exit")
	rootCmd.PersistentFlags().BoolP("version", "v", false, "[deprecated] Print the version and exit")
	rootCmd.PersistentFlags().String("check", "",
		"[deprecated] Run a specific check and print the results. Choose from: process, rtprocess, container, rtcontainer, connections, process_discovery")

	if winParams {
		// windows-specific options for controlling the service
		rootCmd.PersistentFlags().BoolVar(&globalParams.WinParams.StartService, "start-service", false, "Starts the process agent service")
		rootCmd.PersistentFlags().BoolVar(&globalParams.WinParams.StopService, "stop-service", false, "Stops the process agent service")
		rootCmd.PersistentFlags().BoolVar(&globalParams.WinParams.Foreground, "foreground", false, "Always run foreground instead whether session is interactive or not")
	}
	// github.com/fatih/color sets its global color.NoColor to a default value based on
	// whether the process is running in a tty.  So, we only want to override that when
	// the value is true.
	var noColorFlag bool
	rootCmd.PersistentFlags().BoolVarP(&noColorFlag, "no-color", "n", false, "disable color output")
	rootCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if noColorFlag {
			color.NoColor = true
		}
	}

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			rootCmd.AddCommand(cmd)
		}
	}

	return rootCmd
}

// BootstrapConfig is a helper for process-agent config initialization (until we further refactor to use components)
func BootstrapConfig(globalParams *GlobalParams) error {
	if err := pconfig.LoadConfigIfExists(globalParams.ConfFilePath); err != nil {
		return err
	}

	return config.SetupLogger(
		LoggerName,
		config.Datadog.GetString("log_level"),
		"",
		"",
		false,
		true,
		false,
	)
}
