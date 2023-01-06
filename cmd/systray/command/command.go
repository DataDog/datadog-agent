// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `systray` binary, including its subcommands.
package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/systray"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"go.uber.org/fx"
)

const (
	defaultLogFile = "c:\\programdata\\datadog\\logs\\ddtray.log"
)

var (
	// set by the build task and used to configure the logger to output to console when debugging.
	// This value should correspond to the subsystem in the PE header.
	//
	// There will only be console output if the PE subsystem is "console", but the GUI functions will
	// also fail, so this is really only useful for debugging setup/initialization or cmdline setup.
	subsystem = "windows"
)

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand() *cobra.Command {
	systrayParams := systray.BundleParams{}

	// log file path
	var logFilePath string
	confPath, err := winutil.GetProgramDataDir()
	if err == nil {
		logFilePath = filepath.Join(confPath, "logs", "ddtray.log")
	} else {
		logFilePath = defaultLogFile
	}

	// log params
	var logParams log.Params
	if subsystem == "windows" {
		logParams = log.LogForDaemon("TRAY", "log_file", logFilePath)
	} else if subsystem == "console" {
		logParams = log.LogForOneShot("TRAY", "log_file", true)
	}

	// root command
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s", os.Args[0]),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args[]string) error {
			return fxutil.Run(
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewParams(common.DefaultConfPath),
					LogParams: logParams,
				}),
				core.Bundle,
				fx.Supply(systrayParams),
				systray.Bundle,
			)
		},
	}

	//
	// NOTE: The command line help/usage will not be visible in the release binary because the PE subsystem is "windows"
	//

	cmd.PersistentFlags().BoolVar(&systrayParams.LaunchGuiFlag, "launch-gui", false, "Launch browser configuration and exit")

	// launch-elev=true only means the process should have been elevated so that it will not elevate again. If the
	// parameter is specified but the process is not elevated, some operation will fail due to access denied.
	cmd.PersistentFlags().BoolVar(&systrayParams.LaunchElevatedFlag, "launch-elev", false, "Launch program as elevated, internal use only")

	// If this parameter is specified, the process will try to carry out the command before the message loop.
	cmd.PersistentFlags().StringVar(&systrayParams.LaunchCommand, "launch-cmd", "", "Carry out a specific command after launch")

	return cmd
}
