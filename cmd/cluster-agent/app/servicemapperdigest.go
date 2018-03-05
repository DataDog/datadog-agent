package app

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/spf13/cobra"
	"io/ioutil"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"

	"bytes"
	"fmt"
)

func init() {
	ClusterAgentCmd.AddCommand(svcMapperCmd)
}

// TODO add all registered nodes command
var svcMapperCmd = &cobra.Command{
	Use:   "svcmap",
	Short: "Print the map between the services and the pods behind them",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confPath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		err = getServiceMap(args[0]) // if arg[0] == "*", call all.
		if err != nil {
			return err
		}
		return nil
	},
}

func getServiceMap(nodeName string) error {
	var e error
	var s string
	c := util.GetClient(false) // FIX: get certificates right then make this true
	// TODO use https
	urlstr := fmt.Sprintf("http://localhost:%v/api/v1/metadata/%s/*", config.Datadog.GetInt("clusteragent_cmd_port"), nodeName)

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoGet(c, urlstr)
	if e != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if err, found := errMap["error"]; found {
			e = fmt.Errorf(err)
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the map of services to pods and contact support if you continue having issues. \n", e)
		return e
	}

	// The rendering is done in the client so that the agent has less work to do
	if prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ")
		s = prettyJSON.String()
	} else if jsonStatus {
		s = string(r)
	} else {
		formatterServiceMap, err := status.FormatServiceMapCLI(r)
		if err != nil {
			return err
		}
		s = formatterServiceMap
	}

	if statusFilePath != "" {
		ioutil.WriteFile(statusFilePath, []byte(s), 0644)
	} else {
		fmt.Println(s)
	}
	return nil
}
