// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/input"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	cpuProfURL = fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/profile?seconds=120",
		config.Datadog.GetString("expvar_port"))
	heapProfURL = fmt.Sprintf("http://127.0.0.1:%s/debug/pprof/heap?debug=2",
		config.Datadog.GetString("expvar_port"))

	customerEmail   string
	autoconfirm     bool
	forceLocal      bool
	enableProfiling bool
)

func init() {
	AgentCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.Flags().BoolVarP(&forceLocal, "local", "l", false, "Force the creation of the flare by the command line instead of the agent process (useful when running in a containerized env)")
	flareCmd.Flags().BoolVarP(&enableProfiling, "profile", "p", false, "Add performance enableProfiling data to the flare. If used, the flare command will wait for 120s to collect enableProfiling data.")
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

func makeFlare(caseID string) error {
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	var tempDir, hostname string
	var err error
	if forceLocal {
		tempDir, hostname, err = createArchive(logFile)
	} else {
		tempDir, hostname, err = requestArchive(logFile)
	}

	defer os.RemoveAll(tempDir)

	if err != nil {
		return err
	}

	if enableProfiling {
		fmt.Fprintln(color.Output, color.BlueString("Creating a 120s performance profile."))
		if err := writePerformanceProfile(tempDir, hostname); err != nil {
			fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Could not collect performance profile: %s", err)))
			return err
		}
	}

	zipFilePath, err := flare.ZipArchive(flare.GetArchivePath(), tempDir, hostname)
	if err != nil {
		return err
	}

	if _, err := os.Stat(zipFilePath); err != nil {
		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("The flare zipfile \"%s\" does not exist.", zipFilePath)))
		fmt.Fprintln(color.Output, color.RedString("If the agent running in a different container try the '--local' option to generate the flare locally"))
		return err
	}

	fmt.Fprintln(color.Output, fmt.Sprintf("%s is going to be uploaded to Datadog", color.YellowString(zipFilePath)))
	if !autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintln(color.Output, fmt.Sprintf("Aborting. (You can still use %s)", color.YellowString(zipFilePath)))
			return nil
		}
	}

	response, e := flare.SendFlare(zipFilePath, caseID, customerEmail)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}

func writePerformanceProfile(tempDir, hostname string) error {
	// Two heap profiles for diff
	err := flare.WriteHTTPCallContent(tempDir, hostname, "heap_profile.log", heapProfURL)
	if err != nil {
		return err
	}

	err = writeCPUProfile(tempDir, hostname, "cpu.pprof", cpuProfURL)
	if err != nil {
		return err
	}

	err = flare.WriteHTTPCallContent(tempDir, hostname, "heap_profile.log", heapProfURL)
	if err != nil {
		return err
	}

	return nil
}

func requestArchive(logFile string) (string, string, error) {
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

	res, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	if e != nil {
		if res != nil && string(res) != "" {
			fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while making the flare: %s", color.RedString(string(res))))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make the flare. (is it running?)"))
		}
		return createArchive(logFile)
	}

	var filePath flare.Path
	if err := json.Unmarshal(res, &filePath); err != nil {
		fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while decoding the flare file path: %s", color.RedString(err.Error())))
	}

	return filePath["tempDir"], filePath["hostname"], nil
}

func createArchive(logFile string) (string, string, error) {
	fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally."))
	tempdDir, hostname, e := flare.CreateArchive(forceLocal, common.GetDistPath(), common.PyChecksPath, logFile)
	if e != nil {
		fmt.Printf("The flare directory failed to be created: %s\n", e)
		return tempdDir, hostname, e
	}
	return tempdDir, hostname, nil
}

func writeCPUProfile(tempDir, hostname, filename, url string) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}

	path := filepath.Join(tempDir, hostname, filename)
	err = flare.EnsureParentDirsExist(path)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, res.Body)
	return err
}
