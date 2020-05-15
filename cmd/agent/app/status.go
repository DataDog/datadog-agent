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
	"net/http"
	"os"
	"strconv"
	"time"

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

		return requestStatus()
	},
}

var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Print the component status",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfigWithoutSecrets(confFilePath, "")
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
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/status", ipcAddress, config.Datadog.GetInt("cmd_port"))
	r, err := makeRequest(urlstr)
	if err != nil {
		return err
	}
	// attach trace-agent status, if obtainable
	temp := make(map[string]interface{})
	if err := json.Unmarshal(r, &temp); err == nil {
		temp["apmStats"] = getAPMStatus()
		if newr, err := json.Marshal(temp); err == nil {
			r = newr
		}
	}

	// The rendering is done in the client so that the agent has less work to do
	if prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
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
		ioutil.WriteFile(statusFilePath, []byte(s), 0644) //nolint:errcheck
	} else {
		fmt.Println(s)
	}

	return nil
}

// getAPMStatus returns a set of key/value pairs summarizing the status of the trace-agent.
// If the status can not be obtained for any reason, the returned map will contain an "error"
// key with an explanation.
func getAPMStatus() map[string]interface{} {
	port := 8126
	// TODO(gbbr): This should be handled by the shared config package once
	// we migrate APM env. vars there.
	if p, ok := os.LookupEnv("DD_APM_RECEIVER_PORT"); ok {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	if config.Datadog.IsSet("apm_config.receiver_port") {
		port = config.Datadog.GetInt("apm_config.receiver_port")
	}
	url := fmt.Sprintf("http://localhost:%d/debug/vars", port)
	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(url)
	if err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}
	defer resp.Body.Close()
	status := make(map[string]interface{})
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}
	return status
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
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
		s = prettyJSON.String()
	} else {
		s = string(r)
	}

	if statusFilePath != "" {
		ioutil.WriteFile(statusFilePath, []byte(s), 0644) //nolint:errcheck
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
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if err, found := errMap["error"]; found {
			e = fmt.Errorf(err)
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues. \n", e)
		return nil, e
	}

	return r, nil

}
