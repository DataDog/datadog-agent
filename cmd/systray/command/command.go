// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

// Package command implements the top-level `systray` binary, including its subcommands.
package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/systray/systray"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"
)

const (
	defaultLogFile = "c:\\programdata\\datadog\\logs\\ddtray.log"
)

var (
	// set by the build task and used to configure the logger to output to console when debugging.
	// This value should correspond to the subsystem in the PE header.
	//
	// In normal circumstances, we don't want the systray to launch a console window when it runs
	// so the default subsystem is "windows". However, console output can be useful for debugging.
	// Console output can be viewd by setting the PE subsystem to "console".
	subsystem = "windows"
)

func init() {
	// disable cobra mouse trap so cobra doesn't immediately kill our GUI app
	cobra.MousetrapHelpText = ""
}

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand() *cobra.Command {
	systrayParams := systray.Params{}

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
		logParams = log.LogForDaemon("TRAY", "system_tray.log_file", logFilePath)
	} else if subsystem == "console" {
		logParams = log.LogForOneShot("TRAY", "info", true)
	}

	// root command
	cmd := &cobra.Command{
		Use:          fmt.Sprintf("%s", os.Args[0]),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if we are elevated and elevate if necessary. Elevation is required prior to component initialization
			// because of restricted permissions to the agent configuration file.
			err := ensureElevated(systrayParams)
			if err != nil {
				return err
			}

			return fxutil.Run(
				// core
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewParams(common.DefaultConfPath),
					LogParams:    logParams,
				}),
				core.Bundle,
				// flare
				flare.Module,
				// systray
				fx.Supply(systrayParams),
				systray.Module,
				// require the systray component, causing it to start
				fx.Invoke(func(_ systray.Component) {}),
			)
		},
	}

	//
	// NOTE: The command line help/usage will not be visible in the release binary because the PE subsystem is "windows"
	//

	cmd.PersistentFlags().BoolVar(&systrayParams.LaunchGuiFlag, "launch-gui", false, "Launch browser configuration and exit")

	// launch-elev=true only means the process should be elevated so that it will not elevate again. If the
	// parameter is specified but the process is not elevated, some operation will fail due to access denied.
	cmd.PersistentFlags().BoolVar(&systrayParams.LaunchElevatedFlag, "launch-elev", false, "Launch program as elevated, internal use only")

	// If this parameter is specified, the process will try to carry out the command before the message loop.
	cmd.PersistentFlags().StringVar(&systrayParams.LaunchCommand, "launch-cmd", "", "Carry out a specific command after launch")

	return cmd
}

func ensureElevated(params systray.Params) error {
	isAdmin, err := winutil.IsUserAnAdmin()
	if err != nil {
		return fmt.Errorf("failed to call IsUserAnAdmin %v", err)
	}

	if isAdmin {
		// user is an admin, allow execution to continue
		return nil
	}

	// user is not an admin
	if params.LaunchElevatedFlag {
		return fmt.Errorf("not running as elevated but elevated flag is set")
	}

	// attempt to launch as admin
	err = relaunchElevated()
	if err != nil {
		return err
	}

	return fmt.Errorf("exiting to allow elevated process to start")
}

// relaunchElevated launch another instance of the current process asking it to carry out a command as admin.
// If the function succeeds, it will quit the process, otherwise the function will return to the caller.
func relaunchElevated() error {
	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()

	// Reconstruct arguments and tell the new process it should be elevated.
	xargs := []string{"--launch-elev=true"}
	if len(os.Args) > 1 {
		xargs = append(xargs, os.Args[1:]...)
	}
	args := strings.Join(xargs, " ")

	verbPtr, _ := windows.UTF16PtrFromString(verb)
	exePtr, _ := windows.UTF16PtrFromString(exe)
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)
	argPtr, _ := windows.UTF16PtrFromString(args)

	var showCmd int32 = 1 //SW_NORMAL

	err := windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		return fmt.Errorf("Failed to launch self as elevated %v", err)
	}
	return nil
}
