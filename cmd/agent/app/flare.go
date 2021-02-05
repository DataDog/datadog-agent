// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/input"
)

var (
	customerEmail string
	autoconfirm   bool
	forceLocal    bool
	profiling     int
)

func init() {
	AgentCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().IntVarP(&profiling, "profile", "p", -1, "Add performance profiling data to the flare. It will collect a heap profile and a CPU profile for the amount of seconds passed to the flag, with a minimum of 30s")
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

		return makeFlare(caseID)
	},
}

func readProfileData(pdata *flare.ProfileData) error {
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from core.", profiling))
	coreDebugURL := fmt.Sprintf("http://127.0.0.1:%s/debug/pprof", config.Datadog.GetString("expvar_port"))
	if err := flare.CreatePerformanceProfile("core", coreDebugURL, profiling, pdata); err != nil {
		return err
	}

	if k := "apm_config.enabled"; config.Datadog.IsSet(k) && !config.Datadog.GetBool(k) {
		return nil
	}
	traceDebugURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof", config.Datadog.GetInt("apm_config.receiver_port"))
	cpusec := 4 // 5s is the default maximum connection timeout on the trace-agent HTTP server
	if v := config.Datadog.GetInt("apm_config.receiver_timeout"); v > 0 {
		if v > profiling {
			// do not exceed requested duration
			cpusec = profiling
		} else {
			// fit within set limit
			cpusec = v - 1
		}
	}
	fmt.Fprintln(color.Output, color.BlueString("Getting a %ds profile snapshot from trace.", cpusec))
	return flare.CreatePerformanceProfile("trace", traceDebugURL, cpusec, pdata)
}

func makeFlare(caseID string) error {
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	jmxLogFile := config.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = common.DefaultJmxLogFile
	}
	logFiles := []string{logFile, jmxLogFile}
	var (
		profile flare.ProfileData
		err     error
	)
	if profiling >= 30 {
		if err := readProfileData(&profile); err != nil {
			fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Could not collect performance profile: %s", err)))
			return err
		}
	} else if profiling != -1 {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Invalid value for profiling: %d. Please enter an integer of at least 30.", profiling)))
		return err
	}

	var filePath string
	if forceLocal {
		filePath, err = createArchive(logFiles, profile)
	} else {
		filePath, err = requestArchive(logFiles, profile)
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

func requestArchive(logFiles []string, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.BlueString("Asking the agent to build the flare archive."))
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error getting IPC address for the agent: %s", err)))
		return createArchive(logFiles, pdata)
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/flare", ipcAddress, config.Datadog.GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error: %s", e)))
		return createArchive(logFiles, pdata)
	}

	p, err := json.Marshal(pdata)
	if err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Error while encoding profile: %s", e)))
		return "", err
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer(p))
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(r))))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
		}
		return createArchive(logFiles, pdata)
	}
	return string(r), nil
}

func createArchive(logFiles []string, pdata flare.ProfileData) (string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	filePath, e := flare.CreateArchive(true, common.GetDistPath(), common.PyChecksPath, logFiles, pdata)
	if e != nil {
		fmt.Printf("The flare zipfile failed to be created: %s\n", e)
		return "", e
	}
	return filePath, nil
}
