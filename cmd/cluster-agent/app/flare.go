// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/spf13/cobra"
)

var (
	customerEmail string
	autoconfirm   bool
)

func init() {
	ClusterAgentCmd.AddCommand(flareCmd)

	flareCmd.Flags().StringVarP(&customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.SetArgs([]string{"caseID"})
}

var flareCmd = &cobra.Command{
	Use:   "flare [caseID]",
	Short: "Collect a flare and send it to Datadog",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confPath)
		if err != nil {
			return err
		}

		caseID := ""
		if len(args) > 0 {
			caseID = args[0]
		}

		// The flare command should not log anything, all errors should be reported directly to the console without the log format
		config.SetupLogger("off", "", "", false, false, "", true, false)
		if customerEmail == "" {
			var err error
			customerEmail, err = flare.AskForEmail()
			if err != nil {
				fmt.Println("Error reading email, please retry or contact support")
				return err
			}
		}

		return requestFlare(caseID)
	},
}

func requestFlare(caseID string) error {
	fmt.Println("Asking the Cluster agent to build the flare archive.")
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("http://localhost:%v/flare", config.Datadog.GetInt("clusteragent_cmd_port"))

	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	var filePath string
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Printf("The agent ran into an error while making the flare: %s\n", string(r))
		} else {
			fmt.Printf("unable to flare %s \n", string(r))
			fmt.Println("The agent was unable to make the flare.")
		}
		fmt.Println("Initiating flare locally.")

		filePath, e = flare.CreateDCAArchive(true, common.GetDistPath(), logFile)
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	} else {
		filePath = string(r)
	}

	fmt.Printf("%s is going to be uploaded to Datadog\n", filePath)
	if !autoconfirm {
		confirmation := flare.AskForConfirmation("Are you sure you want to upload a flare? [Y/N]")
		if !confirmation {
			fmt.Printf("Aborting. (You can still use %s) \n", filePath)
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
