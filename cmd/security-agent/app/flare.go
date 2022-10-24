// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"bytes"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/input"
)

type flareCliParams struct {
	*common.GlobalParams

	customerEmail string
	autoconfirm   bool
}

func FlareCommands(globalParams *common.GlobalParams) []*cobra.Command {
	cliParams := flareCliParams{
		GlobalParams: globalParams,
	}

	flareCmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The flare command should not log anything, all errors should be reported directly to the console without the log format
			err := config.SetupLogger(loggerName, "off", "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			caseID := ""
			if len(args) > 0 {
				caseID = args[0]
			}

			if cliParams.customerEmail == "" {
				var err error
				cliParams.customerEmail, err = input.AskForEmail()
				if err != nil {
					fmt.Println("Error reading email, please retry or contact support")
					return err
				}
			}

			return requestFlare(caseID, &cliParams)
		},
	}

	flareCmd.Flags().StringVarP(&cliParams.customerEmail, "email", "e", "", "Your email")
	flareCmd.Flags().BoolVarP(&cliParams.autoconfirm, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	flareCmd.SetArgs([]string{"caseID"})

	return []*cobra.Command{flareCmd}
}

func requestFlare(caseID string, params *flareCliParams) error {
	fmt.Fprintln(color.Output, color.BlueString("Asking the Security Agent to build the flare archive."))
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/flare", config.Datadog.GetInt("security_agent.cmd_port"))

	logFile := config.Datadog.GetString("security_agent.log_file")

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	sr := string(r)
	var filePath string
	if e != nil {
		if r != nil && sr != "" {
			fmt.Fprintf(color.Output, "The agent ran into an error while making the flare: %s\n", color.RedString(sr))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make a full flare: %s.", e.Error()))
		}
		fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally, some logs will be missing."))
		filePath, e = flare.CreateSecurityAgentArchive(true, logFile, nil, nil)
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	} else {
		filePath = sr
	}

	fmt.Fprintf(color.Output, "%s is going to be uploaded to Datadog\n", color.YellowString(filePath))
	if !params.autoconfirm {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintf(color.Output, "Aborting. (You can still use %s)\n", color.YellowString(filePath))
			return nil
		}
	}

	response, e := flare.SendFlare(filePath, caseID, params.customerEmail)
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}
