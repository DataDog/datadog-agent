// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/legacy"
	yaml "gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
)

var (
	importCmd = &cobra.Command{
		Use:   "import <path_to_datadog.conf>",
		Short: "Import the old datadog.conf and convert to the new format",
		Long:  ``,
		RunE:  doImport,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(importCmd)
}

func doImport(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please provide the path to datadog.conf")
	}

	datadogConfPath := args[0]

	// read the old configuration in memory
	fmt.Println("Reading config data from:", datadogConfPath)
	agentConfig, err := legacy.GetAgentConfig(datadogConfPath)
	if err != nil {
		return fmt.Errorf("unable to read data from %s: %v", datadogConfPath, err)
	}

	// overwrite curren agent configuration with the converted data
	err = legacy.FromAgentConfig(agentConfig)
	if err != nil {
		return fmt.Errorf("unable to convert configuration data from %s: %v", datadogConfPath, err)
	}

	// backup the original datadog.yaml to datadog.yaml.bak
	var datadogYaml string
	if len(confFilePath) != 0 {
		// the configuration file path was supplied on the command line
		datadogYaml = filepath.Join(confFilePath, "datadog.yaml")
		// config.Datadog.AddConfigPath(confFilePath)
	} else {
		datadogYaml = filepath.Join(common.DefaultConfPath, "datadog.yaml")
	}
	err = os.Rename(datadogYaml, datadogYaml+".bak")
	if err != nil {
		return fmt.Errorf("unable to create a backup for the existing file: %s", datadogYaml)
	}

	// dump the current configuration to datadog.yaml
	b, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return fmt.Errorf("unable to unmarshal config to YAML: %v", err)
	}
	fmt.Println("Writing new configuration file to:", datadogYaml)
	if err = ioutil.WriteFile(datadogYaml, b, 0644); err != nil {
		return fmt.Errorf("unable to unmarshal config to %s: %v", datadogYaml, err)
	}

	return nil
}
