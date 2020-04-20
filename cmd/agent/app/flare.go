// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/input"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	customerEmail string
	autoconfirm   bool
	debugLog      bool
	forceLocal    bool
	delay         int
)

func init() {
	AgentCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().BoolVarP(&debugLog, "debug-log", "d", false, "Switch the log level to debug during the flare, if used the flare command will wait for 30 seconds (it can be overridden using the delay option) to collect log at the debug level")
	flareCmd.Flags().IntVarP(&delay, "delay", "w", 30, "Amount of time (in seconds) to wait for debug log collection when the --enable-debug-log option is used (default: 30)")

	flareCmd.SetArgs([]string{"caseID"})
}

var flareCmd = &cobra.Command{
	Use:   "flare [caseID]",
	Short: "Collect a flare and send it to Datadog",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		err = config.SetupLogger(loggerName, "off", "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		caseID := ""
		if len(args) > 0 {
			caseID = args[0]
		}

		if customerEmail == "" {
			var err error
			customerEmail, err = input.AskForEmail()
			if err != nil {
				fmt.Println("Error reading email, please retry or contact support")
				return err
			}
		}
		if debugLog {
			if e := switchLogLevelAndWait(); e != nil {
				fmt.Printf("Error while changing log level: %v\n", e)
			}
		}
		return makeFlare(caseID)
	},
}

func switchLogLevelAndWait() error {
	err := util.SetAuthToken()
	if err != nil {
		return err
	}
	currentLogLevel, err := getConfigValue("log_level")
	if err != nil {
		return err
	}
	defer setConfigValue("log_level", currentLogLevel)

	// Setup a channel to catch OS signals as it very likely that some users will do CTRL+C while waiting for the delay
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Reset(os.Interrupt, syscall.SIGTERM)
	go func() {
		_ = <-signalCh
		err = setConfigValue("log_level", currentLogLevel)
		if err != nil {
			fmt.Fprintln(color.Output, color.RedString("The flare command was interrupted, setting the log_level to its original value: %v failed because: %v"), currentLogLevel, err)
		}
		os.Exit(1)
	}()

	err = setConfigValue("log_level", "debug")
	if err != nil {
		return err
	}

	fmt.Fprintln(color.Output, color.BlueString("Waiting %v seconds to get debug logs...", delay))
	time.Sleep(time.Duration(delay) * time.Second)

	return nil
}

func makeFlare(caseID string) error {
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	var filePath string
	var err error
	if forceLocal {
		filePath, err = createArchive(logFile)
	} else {
		filePath, err = requestArchive(logFile)
	}

	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", filePath)))
		fmt.Fprintln(color.Output, color.RedString("If the agent running in a different container try the '--local' option to generate the flare locally"))
		return err
	}

	fmt.Fprintln(color.Output, fmt.Sprintf("%s is going to be uploaded to Datadog", color.YellowString(filePath)))
	if !autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintln(color.Output, fmt.Sprintf("Aborting. (You can still use %s)", color.YellowString(filePath)))
			return nil
		}
	}

	response, e := flare.SendFlare(filePath, caseID, customerEmail)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func requestArchive(logFile string) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFile)
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/flare", ipcAddress, config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFile)
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(r))))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
		}
		return createArchive(logFile)
	}
	return string(r), nil
}

func createArchive(logFile string) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, e := flare.CreateArchive(true, common.GetDistPath(), common.PyChecksPath, logFile)
	if e != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", e)
		return "", e
	}
	return filePath, nil
}
