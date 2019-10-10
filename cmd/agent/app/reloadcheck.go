// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"fmt"

	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// TODO: re-enable when the API endpoint is implemented
	// AgentCmd.AddCommand(reloadCheckCommand)
	checkCmd.SetArgs([]string{"checkName"})
}

var reloadCheckCommand = &cobra.Command{
	Use:   "reload-check <check_name>",
	Short: "Reload a running check",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		var checkName string
		if len(args) != 0 {
			checkName = args[0]
		} else {
			return fmt.Errorf("missing arguments")
		}

		return doReloadCheck(checkName)
	},
}

// reload check
func doReloadCheck(checkName string) error {
	if checkName == "" {
		return fmt.Errorf("Must supply a check name to query")
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/check/%s/reload", ipcAddress, config.Datadog.GetInt("cmd_port"), checkName)

	postbody := ""

	body, e := util.DoPost(c, urlstr, "application/json", strings.NewReader(postbody))
	if e != nil {
		return fmt.Errorf("error getting check status for check %s: %v", checkName, e)
	}

	fmt.Printf("Reload check %s: %s\n", checkName, body)
	return nil
}
