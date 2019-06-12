// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	jsonStatus      bool
	prettyPrintJSON bool
	statusFilePath  string
)

func init() {
	AgentCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVarP(&jsonStatus, "json", "j", false, "print out raw json")
	statusCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	statusCmd.Flags().StringVarP(&statusFilePath, "file", "o", "", "Output the status command to a file")
	statusCmd.AddCommand(componentCmd)
	componentCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	componentCmd.Flags().StringVarP(&statusFilePath, "file", "o", "", "Output the status command to a file")
}

var statusCmd = &cobra.Command{
	Use:   "status [component [name]]",
	Short: "Print the current status",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfigWithoutSecrets(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}
		return requestStatus()
	},
}

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Print the component status",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfigWithoutSecrets(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}

		if len(args) != 1 {
			return fmt.Errorf("a component name must be specified")
		}
		return componentStatus(args[0])
	},
}

func requestStatus() error {
	var s string

	if !prettyPrintJSON && !jsonStatus {
		fmt.Printf("Getting the status from the agent.\n\n")
	}
	urlstr := fmt.Sprintf("https://localhost:%v/agent/status", config.Datadog.GetInt("cmd_port"))

	r, err := makeRequest(urlstr)
	if err != nil {
		return err
	}

	// The rendering is done in the client so that the agent has less work to do
	if prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ")
		s = prettyJSON.String()
	} else if jsonStatus {
		s = string(r)
	} else {
		formattedStatus, err := status.FormatStatus(r)
		if err != nil {
			return err
		}
		s = formattedStatus
	}

	if statusFilePath != "" {
		ioutil.WriteFile(statusFilePath, []byte(s), 0644)
	} else {
		fmt.Println(s)
	}

	return nil
}

func componentStatus(component string) error {
	var s string

	urlstr := fmt.Sprintf("https://localhost:%v/agent/%s/status", config.Datadog.GetInt("cmd_port"), component)

	r, err := makeRequest(urlstr)
	if err != nil {
		return err
	}

	// The rendering is done in the client so that the agent has less work to do
	if prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ")
		s = prettyJSON.String()
	} else {
		s = string(r)
	}

	if statusFilePath != "" {
		ioutil.WriteFile(statusFilePath, []byte(s), 0644)
	} else {
		fmt.Println(s)
	}

	return nil
}

func makeRequest(url string) ([]byte, error) {
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return nil, e
	}

	r, e := util.DoGet(c, url)
	if e != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if err, found := errMap["error"]; found {
			e = fmt.Errorf(err)
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues. \n", e)
		return nil, e
	}

	return r, nil

}
