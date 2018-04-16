// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
	ClusterAgentCmd.AddCommand(metaMapperCmd)
}

var metaMapperCmd = &cobra.Command{
	Use:   "metamap [nodeName]",
	Short: "Print the map between the metadata and the pods associated",
	Long: `The metamap command is mostly designed for troubleshooting purposes.
One can easily identify which pods are running on which nodes,
as well as which services are serving the pods. Or the deployment name for the pod`,
	Example: "datadog-cluster-agent metamap ip-10-0-115-123",
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
		if len(args) > 0 {
			nodeName = args[0]
		}
		return getMetadataMap(nodeName) // if nodeName == "", call all.
	},
}

func getMetadataMap(nodeName string) error {
	var e error
	var s string
	c := util.GetClient(false) // FIX: get certificates right then make this true
	var urlstr string
	if nodeName == "" {
		urlstr = fmt.Sprintf("https://localhost:%v/api/v1/metadata", config.Datadog.GetInt("cmd_port"))
	} else {
		urlstr = fmt.Sprintf("https://localhost:%v/api/v1/metadata/%s", config.Datadog.GetInt("cmd_port"), nodeName)
	}

	// Set session token
	e = util.SetDCAAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoGetExternalEndpoint(c, urlstr)
	if e != nil {
		fmt.Printf(`
		Could not reach agent: %v
		Make sure the agent is properly running before requesting the map of services to pods.
		Contact support if you continue having issues.`, e)
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
		formattedMetadataMap, err := status.FormatMetadataMapCLI(r)
		if err != nil {
			return err
		}
		s = formattedMetadataMap
	}

	if statusFilePath != "" {
		ioutil.WriteFile(statusFilePath, []byte(s), 0644)
	} else {
		fmt.Println(s)
	}
	return nil
}
