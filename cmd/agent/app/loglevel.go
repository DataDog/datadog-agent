// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(logLevelCmd)
	logLevelCmd.Flags().StringVarP(&logLevel, "set", "s", "", "Set the log level value. Possible values: trace, debug, info, warn, error, critical")
}

var logLevelCmd = &cobra.Command{
	Use:   "loglevel",
	Short: "Set the log level of a running Agent",
	Long: `Set the log level of a running Agent

The value set using this command will override the value set in the configuration
while the Agent is running, however, this new value is not saved: it is reset
to its configuration value when the Agent is restarted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}
		if err = changeLogLevel(); err != nil {
			return err
		}
		return nil
	},
}

func changeLogLevel() error {
	fmt.Printf("Setting the Agent log level.\n\n")
	var err error
	client := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/log-level", config.Datadog.GetInt("cmd_port"))

	// Set session token
	if err = util.SetAuthToken(); err != nil {
		return err
	}

	if len(logLevel) == 0 {
		return fmt.Errorf("invalid log level value")
	}

	body := fmt.Sprintf("loglevel=%s", html.EscapeString(logLevel))
	buff := bytes.NewBuffer([]byte(body))
	resp, err := util.DoPost(client, urlstr, "application/x-www-form-urlencoded", buff)

	if err != nil {
		var errMap = make(map[string]string)
		if jsonErr := json.Unmarshal(resp, &errMap); jsonErr != nil {
			fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Could not read agent response: %v \n", jsonErr)))
			fmt.Fprintln(color.Output, "Make sure the agent is running before trying to set the log level and contact support if you continue having issues.")
			return fmt.Errorf("%s", jsonErr)
		}

		// If the error has been marshalled into a json object, check it and return it properly
		if v, found := errMap["error"]; found {
			err = fmt.Errorf(v)
		}

		fmt.Fprintln(color.Output, color.RedString(fmt.Sprintf("Agent returned an error: %v \n", err)))
		fmt.Fprintln(color.Output, "Make sure the agent is running before trying to set the log level and contact support if you continue having issues.")
		return err
	}

	fmt.Fprintln(color.Output, color.GreenString(fmt.Sprintf("Log level successfully set to: %s", string(resp))))
	return nil
}
