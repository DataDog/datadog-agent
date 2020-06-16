// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	statusCmd = &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE:  runStatus,
	}

	statusArgs = struct {
		json            bool
		prettyPrintJSON bool
		file            string
	}{}
)

func init() {
	SecurityAgentCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVarP(&statusArgs.json, "json", "j", false, "print out raw json")
	statusCmd.Flags().BoolVarP(&statusArgs.prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	statusCmd.Flags().StringVarP(&statusArgs.file, "file", "o", "", "Output the status command to a file")
}

func runStatus(cmd *cobra.Command, args []string) error {
	if flagNoColor {
		color.NoColor = true
	}

	// we'll search for a config file named `datadog.yaml`
	config.Datadog.SetConfigName("datadog")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global security agent configuration: %v", err)
	}

	err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		return log.Errorf("Cannot setup logger, exiting: %v", err)
	}

	return requestStatus()
}

func requestStatus() error {
	fmt.Printf("Getting the status from the agent.\n")
	var e error
	var s string
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/status", config.Datadog.GetInt("compliance_config.cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoGet(c, urlstr)
	if e != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if err, found := errMap["error"]; found {
			e = fmt.Errorf(err)
		}

		fmt.Printf(`
		Could not reach security agent: %v
		Make sure the agent is running before requesting the status.
		Contact support if you continue having issues.`, e)
		return e
	}

	// The rendering is done in the client so that the agent has less work to do
	if statusArgs.prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
		s = prettyJSON.String()
	} else if statusArgs.json {
		s = string(r)
	} else {
		formattedStatus, err := status.FormatSecurityAgentStatus(r)
		if err != nil {
			return err
		}
		s = formattedStatus
	}

	if statusArgs.file != "" {
		ioutil.WriteFile(statusArgs.file, []byte(s), 0644) //nolint:errcheck
	} else {
		fmt.Println(s)
	}

	return nil
}
