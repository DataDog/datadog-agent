package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
)

func init() {
	ClusterAgentCmd.AddCommand(svcMapperCmd)
	svcMapperCmd.SetArgs([]string{"caseID"})
}

// TODO add all registered nodes command
var svcMapperCmd = &cobra.Command{
	Use:   "svcmap [nodeName]",
	Short: "Print the map between the services and the pods behind them",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFound := false
		// a path to the folder containing the config file was passed
		if len(confPath) != 0 {
			// we'll search for a config file named `datadog-cluster.yaml`
			config.Datadog.AddConfigPath(confPath)
			confErr := config.Datadog.ReadInConfig()
			if confErr != nil {
				log.Error(confErr)
			} else {
				configFound = true
			}
		}
		if !configFound {
			log.Debugf("Config read from env variables")
		}
		nodeName := ""
		// "*" Can be used to output all nodes.
		if len(args) == 0 {
			log.Infof("You need to specify on which node you want to run the service mapper.")
			return nil
		}
		nodeName = args[0]
		err := getServiceMap(nodeName) // if nodeName == "*", call all.
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
		formattedServiceMap, err := status.FormatServiceMapCLI(r)
		if err != nil {
			return err
		}
		s = formattedServiceMap
	}

	if statusFilePath != "" {
		ioutil.WriteFile(statusFilePath, []byte(s), 0644)
	} else {
		fmt.Println(s)
	}
	return nil
}
