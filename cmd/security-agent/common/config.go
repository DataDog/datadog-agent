// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// MergeConfigurationFiles reads an array of configuration filenames and attempts to merge them. The userDefined value is used to specify that configurationFilesArray contains filenames defined on the command line
func MergeConfigurationFiles(configName string, configurationFilesArray []string, userDefined bool) error {
	// we'll search for a config file named `datadog.yaml`
	coreconfig.Datadog.SetConfigName(configName)

	// Track if a configuration file was loaded
	loadedConfiguration := false

	// Load and merge configuration files
	for _, configurationFilename := range configurationFilesArray {
		if _, err := os.Stat(configurationFilename); err != nil {
			if userDefined {
				fmt.Printf("Warning: unable to access %s\n", configurationFilename)
			}
			continue
		}
		if loadedConfiguration == false {
			err := common.SetupConfig(configurationFilename)
			if err != nil {
				if userDefined {
					fmt.Printf("Warning: unable to open %s\n", configurationFilename)
				}
				continue
			}
			loadedConfiguration = true
		} else {
			file, err := os.Open(configurationFilename)
			if err != nil {
				if userDefined {
					fmt.Printf("Warning: unable to open %s\n", configurationFilename)
				}
				continue
			}

			err = coreconfig.Datadog.MergeConfig(file)
			if err != nil {
				return fmt.Errorf("Unable to merge a configuration file: %v", err)
			}
		}
	}

	if loadedConfiguration == false {
		return fmt.Errorf("Unable to load any configuration file from %s", configurationFilesArray)
	}

	return nil
}
