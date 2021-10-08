// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	filters diagnostic.Filters
)

func init() {
	AgentCmd.AddCommand(troubleshootLogsCmd)
	troubleshootLogsCmd.Flags().StringVar(&filters.Name, "name", "", "Filter by name")
	troubleshootLogsCmd.Flags().StringVar(&filters.Type, "type", "", "Filter by type")
	troubleshootLogsCmd.Flags().StringVar(&filters.Source, "source", "", "Filter by source")
}

var troubleshootLogsCmd = &cobra.Command{
	Use:   "stream-logs",
	Short: "Stream the logs being processed by a running agent",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return connectAndStream()
	},
}

func connectAndStream() error {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}

	body, err := json.Marshal(&filters)

	if err != nil {
		return err
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/stream-logs", ipcAddress, config.Datadog.GetInt("cmd_port"))
	return streamRequest(urlstr, body, func(chunk []byte) {
		fmt.Print(string(chunk))
	})
}

func streamRequest(url string, body []byte, onChunk func([]byte)) error {
	var e error
	c := util.GetClient(false)

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	e = util.DoPostChunked(c, url, "application/json", bytes.NewBuffer(body), onChunk)

	if e == io.EOF {
		return nil
	}
	if e != nil {
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the logs and contact support if you continue having issues. \n", e)
	}
	return e
}
