// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/util/input"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	dsdStatsFilePath string
)

func init() {
	AgentCmd.AddCommand(dogstatsdStatsCmd)
	dogstatsdStatsCmd.Flags().BoolVarP(&jsonStatus, "json", "j", false, "print out raw json")
	dogstatsdStatsCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	dogstatsdStatsCmd.Flags().StringVarP(&dsdStatsFilePath, "file", "o", "", "Output the dogstatsd-stats command to a file")
}

var dogstatsdStatsCmd = &cobra.Command{
	Use:   "dogstatsd-stats",
	Short: "Print basic statistics on the metrics processed by dogstatsd",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return requestDogstatsdStats()
	},
}

func requestDogstatsdStats() error {
	fmt.Printf("Getting the dogstatsd stats from the agent.\n\n")
	var e error
	var s string
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/dogstatsd-stats", ipcAddress, config.Datadog.GetInt("cmd_port"))

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

		if len(errMap["error_type"]) > 0 {
			fmt.Println(e)
			return nil
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the dogstatsd stats and contact support if you continue having issues. \n", e)

		return e
	}

	// The rendering is done in the client so that the agent has less work to do
	if prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
		s = prettyJSON.String()
	} else if jsonStatus {
		s = string(r)
	} else {
		s, e = dogstatsd.FormatDebugStats(r)
		if e != nil {
			fmt.Printf("Could not format the statistics, the data must be inconsistent. You may want to try the JSON output. Contact the support if you continue having issues.\n")
			return nil
		}
	}

	if dsdStatsFilePath == "" {
		fmt.Println(s)
		return nil
	}

	// if the file is already existing, ask for a confirmation.
	if _, err := os.Stat(dsdStatsFilePath); err == nil {
		if !input.AskForConfirmation(fmt.Sprintf("'%s' already exists, do you want to overwrite it? [y/N]", dsdStatsFilePath)) {
			fmt.Println("Canceling.")
			return nil
		}
	}

	if err := ioutil.WriteFile(dsdStatsFilePath, []byte(s), 0644); err != nil {
		fmt.Println("Error while writing the file (is the location writable by the dd-agent user?):", err)
	} else {
		fmt.Println("Dogstatsd stats written in:", dsdStatsFilePath)
	}

	return nil
}
