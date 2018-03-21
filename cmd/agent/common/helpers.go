// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package common

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) error {
	// Grab the environment variable
	envConfigPath := os.Getenv("DD_CONFIG_PATH")

	// Check the environment config path first,
	// the confFilePath from the cli should override
	// so don't add it to the config paths until the second step
	if len(envConfigPath) != 0 {
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(envConfigPath, ".yaml") {
			config.Datadog.SetConfigFile(envConfigPath)
		}
	}

	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") {
			config.Datadog.SetConfigFile(confFilePath)
		}
	}

	// set the paths where a config file is expected
	if len(envConfigPath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(envConfigPath)
	}

	config.Datadog.AddConfigPath(DefaultConfPath)

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		return fmt.Errorf("unable to load Datadog config file: %s", err)
	}
	return nil
}
