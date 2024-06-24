// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `process-agent` binary, including its subcommands.
package command

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	logComponentimpl "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

//nolint:revive // TODO(PROC) Fix revive linter
const LoggerName config.LoggerName = "PROCESS"

// DaemonLogParams are the log params should be given to the `core.BundleParams` for when the process agent is running as a daemon
var DaemonLogParams = logComponentimpl.ForDaemon(string(LoggerName), "process_config.log_file", config.DefaultProcessAgentLogFile)

// OneShotLogParams are the log params that are given to commands
var OneShotLogParams = logComponentimpl.ForOneShot(string(LoggerName), "info", true)

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

	// WinParams provides windows specific options
	WinParams WinParams

	// NoColor is a flag to disable color output
	NoColor bool
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
	globalParams := GlobalParams{
		// github.com/fatih/color sets its global color.NoColor to a default value based on
		// whether the process is running in a tty
		NoColor: color.NoColor,
	}

	rootCmd := &cobra.Command{
		Run: func(_ *cobra.Command, _ []string) {
			rootCmdRun(&globalParams)
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&globalParams.ConfFilePath, flags.CfgPath, flags.DefaultConfPath, "Path to datadog.yaml config")

	if flags.DefaultSysProbeConfPath != "" {
		rootCmd.PersistentFlags().StringVar(&globalParams.SysProbeConfFilePath, flags.SysProbeConfig, flags.DefaultSysProbeConfPath, "Path to system-probe.yaml config")
	}

	rootCmd.PersistentFlags().StringVarP(&globalParams.PidFilePath, "pid", "p", "", "Path to set pidfile for process")
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
	rootCmd.PersistentFlags().BoolVarP(&globalParams.NoColor, "no-color", "n", globalParams.NoColor, "disable color output")
	rootCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if globalParams.NoColor {
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

// SetHostMountEnv sets HOST_PROC and HOST_SYS mounts if applicable in containerized environments
func SetHostMountEnv(logger logComponent.Component) {
	// Set default values for proc/sys paths if unset.
	// Generally only applicable for container-only cases like Fargate.
	// This is primarily used by gopsutil to correlate cpu metrics with host processes
	if !config.IsContainerized() || !filesystem.FileExists("/host") {
		return
	}

	if v := os.Getenv("HOST_PROC"); v == "" {
		err := os.Setenv("HOST_PROC", "/host/proc")
		if err != nil {
			_ = logger.Error("Failed to set `HOST_PROC` environment variable")
		} else {
			logger.Debug("Set `HOST_PROC` environment variable")
		}
	}
	if v := os.Getenv("HOST_SYS"); v == "" {
		err := os.Setenv("HOST_SYS", "/host/sys")
		if err != nil {
			_ = logger.Error("Failed to set `HOST_SYS` environment variable")
		} else {
			logger.Debug("Set `HOST_SYS` environment variable")
		}
	}
}

//nolint:revive // TODO(PROC) Fix revive linter
func GetCoreBundleParamsForOneShot(globalParams *GlobalParams) core.BundleParams {
	return core.BundleParams{
		ConfigParams:         configComponent.NewAgentParams(globalParams.ConfFilePath),
		SecretParams:         secrets.NewEnabledParams(),
		SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
		LogParams:            OneShotLogParams,
	}
}
