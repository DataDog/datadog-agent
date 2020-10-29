// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// MergeConfigurationFiles reads an array of configuration filenames and attempts to merge them
func MergeConfigurationFiles(configName string, configurationFilesArray []string) error {
	// we'll search for a config file named `datadog.yaml`
	coreconfig.Datadog.SetConfigName(configName)

	for index, configurationFilename := range configurationFilesArray {
		if index == 0 {
			err := common.SetupConfig(configurationFilename)
			if err != nil {
				return fmt.Errorf("Unable to open a configuration file: %v", err)
			}
		} else {
			file, err := os.Open(configurationFilename)
			if err != nil {
				return fmt.Errorf("Unable to open a configuration file: %v", err)
			}

			err = coreconfig.Datadog.MergeConfig(file)
			if err != nil {
				return fmt.Errorf("Unable to merge a configuration file: %v", err)
			}
		}
	}

	return nil
}
