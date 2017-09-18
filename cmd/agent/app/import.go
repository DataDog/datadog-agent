// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"io/ioutil"
	"os"

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

	force = false
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(importCmd)

	// local flags
	importCmd.Flags().BoolVarP(&force, "force", "f", force, "force the creation of the yaml file")
}

func doImport(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please provide the path to datadog.conf")
	}

	datadogConfPath := args[0]

	// read the old configuration in memory
	agentConfig, err := legacy.GetAgentConfig(datadogConfPath)
	if err != nil {
		return fmt.Errorf("unable to read data from %s: %v", datadogConfPath, err)
	}

	// Global Agent configuration
	err = common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// store the current datadog.yaml path
	datadogYamlPath := config.Datadog.ConfigFileUsed()

	if config.Datadog.GetString("api_key") != "" && !force {
		return fmt.Errorf("%s seems to contain a valid configuration, run the command again with --force or -f to overwrite it",
			datadogYamlPath)
	}

	// overwrite current agent configuration with the converted data
	err = legacy.FromAgentConfig(agentConfig)
	if err != nil {
		return fmt.Errorf("unable to convert configuration data from %s: %v", datadogConfPath, err)
	}

	// backup the original datadog.yaml to datadog.yaml.bak
	err = os.Rename(datadogYamlPath, datadogYamlPath+".bak")
	if err != nil {
		return fmt.Errorf("unable to create a backup for the existing file: %s", datadogYamlPath)
	}

	// dump the current configuration to datadog.yaml
	b, err := yaml.Marshal(config.Datadog.AllSettings())
	if err != nil {
		return fmt.Errorf("unable to unmarshal config to YAML: %v", err)
	}
	// file permissions will be used only to create the file if doesn't exist,
	// please note on Windows such permissions have no effect.
	if err = ioutil.WriteFile(datadogYamlPath, b, 0640); err != nil {
		return fmt.Errorf("unable to unmarshal config to %s: %v", datadogYamlPath, err)
	}

	fmt.Printf("Successfully imported the contents of %s into %s\n", datadogConfPath, datadogYamlPath)

	return nil
}
