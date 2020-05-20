// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

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

		if flagNoColor {
			color.NoColor = true
		}

		// we'll search for a config file named `datadog-cluster.yaml`
		config.Datadog.SetConfigName("datadog-cluster")
		err := common.SetupConfig(confPath)
		if err != nil {
			return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
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
		urlstr = fmt.Sprintf("https://localhost:%v/api/v1/tags/pod", config.Datadog.GetInt("cluster_agent.cmd_port"))
	} else {
		urlstr = fmt.Sprintf("https://localhost:%v/api/v1/tags/pod/%s", config.Datadog.GetInt("cluster_agent.cmd_port"), nodeName)
	}

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoGet(c, urlstr)
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
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
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
		ioutil.WriteFile(statusFilePath, []byte(s), 0644) //nolint:errcheck
	} else {
		fmt.Println(s)
	}
	return nil
}
